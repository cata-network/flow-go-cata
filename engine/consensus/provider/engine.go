// (c) 2019 Dapper Labs - ALL RIGHTS RESERVED

package provider

import (
	"fmt"

	"github.com/opentracing/opentracing-go"
	"github.com/rs/zerolog"

	"github.com/dapperlabs/flow-go/engine"
	"github.com/dapperlabs/flow-go/model/flow"
	"github.com/dapperlabs/flow-go/model/flow/filter"
	"github.com/dapperlabs/flow-go/model/messages"
	"github.com/dapperlabs/flow-go/module"
	"github.com/dapperlabs/flow-go/module/metrics"
	"github.com/dapperlabs/flow-go/module/trace"
	"github.com/dapperlabs/flow-go/network"
	"github.com/dapperlabs/flow-go/state/protocol"
	"github.com/dapperlabs/flow-go/utils/logging"
)

// Engine represents the provider engine, used to spread block proposals across
// the flow system, to non-consensus nodes. It makes sense to use a separate
// engine to isolate the consensus algorithm from other processes, which allows
// to create a different underlying protocol for consensus nodes, which have a
// higher priority to receive block proposals, and other nodes
type Engine struct {
	unit    *engine.Unit         // used for concurrency & shutdown
	log     zerolog.Logger       // used to log relevant actions with context
	message module.EngineMetrics // used to track sent & received messages
	tracer  module.Tracer
	con     network.Conduit // used to talk to other nodes on the network
	state   protocol.State  // used to access the  protocol state
	me      module.Local    // used to access local node information
}

// New creates a new block provider engine.
func New(
	log zerolog.Logger,
	message module.EngineMetrics,
	tracer module.Tracer,
	net module.Network,
	state protocol.State,
	me module.Local,
) (*Engine, error) {

	// initialize the propagation engine with its dependencies
	e := &Engine{
		unit:    engine.NewUnit(),
		log:     log.With().Str("engine", "provider").Logger(),
		message: message,
		tracer:  tracer,
		state:   state,
		me:      me,
	}

	// register the engine with the network layer and store the conduit
	con, err := net.Register(engine.BlockProvider, e)
	if err != nil {
		return nil, fmt.Errorf("could not register engine: %w", err)
	}

	e.con = con

	return e, nil
}

// Ready returns a ready channel that is closed once the engine has fully
// started. For the provider engine, we consider the engine up and running
// upon initialization.
func (e *Engine) Ready() <-chan struct{} {
	return e.unit.Ready()
}

// Done returns a done channel that is closed once the engine has fully stopped.
// For the ingestion engine, it only waits for all submit goroutines to end.
func (e *Engine) Done() <-chan struct{} {
	return e.unit.Done()
}

// SubmitLocal submits an event originating on the local node.
func (e *Engine) SubmitLocal(event interface{}) {
	e.Submit(e.me.NodeID(), event)
}

// Submit submits the given event from the node with the given origin ID
// for processing in a non-blocking manner. It returns instantly and logs
// a potential processing error internally when done.
func (e *Engine) Submit(originID flow.Identifier, event interface{}) {
	e.unit.Launch(func() {
		err := e.Process(originID, event)
		if err != nil {
			engine.LogError(e.log, err)
		}
	})
}

// ProcessLocal processes an event originating on the local node.
func (e *Engine) ProcessLocal(event interface{}) error {
	return e.Process(e.me.NodeID(), event)
}

// Process processes the given event from the node with the given origin ID in
// a blocking manner. It returns the potential processing error when done.
func (e *Engine) Process(originID flow.Identifier, event interface{}) error {
	return e.unit.Do(func() error {
		return e.process(originID, event)
	})
}

// process processes the given ingestion engine event. Events that are given
// to this function originate within the provider engine on the node with the
// given origin ID.
func (e *Engine) process(originID flow.Identifier, event interface{}) error {
	switch ev := event.(type) {
	case *messages.BlockProposal:
		return e.onBlockProposal(originID, ev)
	default:
		return fmt.Errorf("invalid event type (%T)", event)
	}
}

// onBlockProposal is used when we want to broadcast a local block to the network.
func (e *Engine) onBlockProposal(originID flow.Identifier, proposal *messages.BlockProposal) error {

	for _, g := range proposal.Payload.Guarantees {
		if span, ok := e.tracer.GetSpan(g.CollectionID, trace.CONProcessCollection); ok {
			childSpan := e.tracer.StartSpan(g.CollectionID, trace.CONProvOnBlockProposal, opentracing.ChildOf(
				span.Context()))
			childSpan.Finish()
		}
	}

	log := e.log.With().
		Hex("origin_id", originID[:]).
		Uint64("block_view", proposal.Header.View).
		Hex("block_id", logging.Entity(proposal.Header)).
		Hex("parent_id", proposal.Header.ParentID[:]).
		Hex("signer", proposal.Header.ProposerID[:]).
		Logger()

	log.Info().Msg("block proposal submitted for propagation")

	// currently, only accept blocks that come from our local consensus
	localID := e.me.NodeID()
	if originID != localID {
		return engine.NewInvalidInputErrorf("non-local block (nodeID: %x)", originID)
	}

	// get all non-consensus nodes in the system
	identities, err := e.state.Final().Identities(filter.Not(filter.HasRole(flow.RoleConsensus)))
	if err != nil {
		return fmt.Errorf("could not get identities: %w", err)
	}

	// submit the blocks to the targets
	err = e.con.Submit(proposal, identities.NodeIDs()...)
	if err != nil {
		return fmt.Errorf("could not broadcast block: %w", err)
	}

	e.message.MessageSent(metrics.EngineConsensusProvider, metrics.MessageBlockProposal)

	log.Info().Msg("block proposal propagated to non-consensus nodes")

	return nil
}
