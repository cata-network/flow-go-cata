// (c) 2021 Dapper Labs - ALL RIGHTS RESERVED

package sealing

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/engine"
	"github.com/onflow/flow-go/engine/consensus/approvals"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module"
	"github.com/onflow/flow-go/module/mempool"
	"github.com/onflow/flow-go/module/trace"
	"github.com/onflow/flow-go/network"
	"github.com/onflow/flow-go/state/protocol"
	"github.com/onflow/flow-go/storage"
	"github.com/onflow/flow-go/utils/logging"
)

// DefaultRequiredApprovalsForSealConstruction is the default number of approvals required to construct a candidate seal
// for subsequent inclusion in block.
const DefaultRequiredApprovalsForSealConstruction = 0

// DefaultEmergencySealingActive is a flag which indicates when emergency sealing is active, this is a temporary measure
// to make fire fighting easier while seal & verification is under development.
const DefaultEmergencySealingActive = false

type Options struct {
	emergencySealingActive               bool   // flag which indicates if emergency sealing is active or not. NOTE: this is temporary while sealing & verification is under development
	requiredApprovalsForSealConstruction uint   // min number of approvals required for constructing a candidate seal
	approvalRequestsThreshold            uint64 // threshold for re-requesting approvals: min height difference between the latest finalized block and the block incorporating a result
}

func DefaultOptions() Options {
	return Options{
		emergencySealingActive:               DefaultEmergencySealingActive,
		requiredApprovalsForSealConstruction: DefaultRequiredApprovalsForSealConstruction,
		approvalRequestsThreshold:            10,
	}
}

// Core is an implementation of ResultApprovalProcessor interface
// This struct is responsible for:
// 	- collecting approvals for execution results
// 	- processing multiple incorporated results
// 	- pre-validating approvals (if they are outdated or non-verifiable)
// 	- pruning already processed collectorTree
type Core struct {
	log                       zerolog.Logger                     // used to log relevant actions with context
	collectorTree             *approvals.AssignmentCollectorTree // levelled forest for assignment collectors
	approvalsCache            *approvals.LruCache                // in-memory cache of approvals that weren't verified
	atomicLastSealedHeight    uint64                             // atomic variable for last sealed block height
	atomicLastFinalizedHeight uint64                             // atomic variable for last finalized block height
	headers                   storage.Headers                    // used to access block headers in storage
	state                     protocol.State                     // used to access protocol state
	seals                     storage.Seals                      // used to get last sealed block
	requestTracker            *approvals.RequestTracker          // used to keep track of number of approval requests, and blackout periods, by chunk
	pendingReceipts           mempool.PendingReceipts            // buffer for receipts where an ancestor result is missing, so they can't be connected to the sealed results
	metrics                   module.ConsensusMetrics            // used to track consensus metrics
	tracer                    module.Tracer                      // used to trace execution
	options                   Options
}

func NewCore(
	log zerolog.Logger,
	tracer module.Tracer,
	conMetrics module.ConsensusMetrics,
	headers storage.Headers,
	state protocol.State,
	sealsDB storage.Seals,
	assigner module.ChunkAssigner,
	verifier module.Verifier,
	sealsMempool mempool.IncorporatedResultSeals,
	approvalConduit network.Conduit,
	options Options,
) (*Core, error) {
	lastSealed, err := state.Sealed().Head()
	if err != nil {
		return nil, fmt.Errorf("could not retrieve last sealed block: %w", err)
	}

	core := &Core{
		log:            log.With().Str("engine", "sealing.Core").Logger(),
		tracer:         tracer,
		metrics:        conMetrics,
		approvalsCache: approvals.NewApprovalsLRUCache(1000),
		headers:        headers,
		state:          state,
		seals:          sealsDB,
		options:        options,
		requestTracker: approvals.NewRequestTracker(10, 30),
	}

	factoryMethod := func(result *flow.ExecutionResult) (*approvals.AssignmentCollector, error) {
		return approvals.NewAssignmentCollector(result, core.state, core.headers, assigner, sealsMempool, verifier,
			approvalConduit, core.requestTracker, options.requiredApprovalsForSealConstruction)
	}

	core.collectorTree = approvals.NewAssignmentCollectorTree(lastSealed, headers, factoryMethod)

	return core, nil
}

func (c *Core) lastSealedHeight() uint64 {
	return atomic.LoadUint64(&c.atomicLastSealedHeight)
}

func (c *Core) lastFinalizedHeight() uint64 {
	return atomic.LoadUint64(&c.atomicLastFinalizedHeight)
}

// processIncorporatedResult implements business logic for processing single incorporated result
// Returns:
// * engine.InvalidInputError - incorporated result is invalid
// * engine.UnverifiableInputError - result is unverifiable since referenced block cannot be found
// * engine.OutdatedInputError - result is outdated for instance block was already sealed
// * exception in case of any other error, usually this is not expected
// * nil - successfully processed incorporated result
func (c *Core) processIncorporatedResult(result *flow.IncorporatedResult) error {
	err := c.checkBlockOutdated(result.Result.BlockID)
	if err != nil {
		return fmt.Errorf("won't process outdated or unverifiable execution result %s: %w", result.Result.BlockID, err)
	}

	incorporatedBlock, err := c.headers.ByBlockID(result.IncorporatedBlockID)
	if err != nil {
		return fmt.Errorf("could not get block height for incorporated block %s: %w",
			result.IncorporatedBlockID, err)
	}
	incorporatedAtHeight := incorporatedBlock.Height

	lastFinalizedBlockHeight := c.lastFinalizedHeight()

	// check if we are dealing with finalized block or an orphan
	if incorporatedAtHeight <= lastFinalizedBlockHeight {
		finalized, err := c.headers.ByHeight(incorporatedAtHeight)
		if err != nil {
			return fmt.Errorf("could not retrieve finalized block at height %d: %w", incorporatedAtHeight, err)
		}
		if finalized.ID() != result.IncorporatedBlockID {
			// it means that we got incorporated result for a block which doesn't extend our chain
			// and should be discarded from future processing
			return engine.NewOutdatedInputErrorf("won't process incorporated result from orphan block %s", result.IncorporatedBlockID)
		}
	}

	// in case block is not finalized we will create collector and start processing approvals
	// no checks for orphans can be made at this point
	// we expect that assignment collector will cleanup orphan IRs whenever new finalized block is processed

	lazyCollector, err := c.collectorTree.GetOrCreateCollector(result.Result)
	if err != nil {
		return fmt.Errorf("could not process incorporated result, cannot create collector: %w", err)
	}

	if !lazyCollector.Processable {
		return engine.NewOutdatedInputErrorf("collector for %s is marked as non processable", result.ID())
	}

	err = lazyCollector.Collector.ProcessIncorporatedResult(result)
	if err != nil {
		return fmt.Errorf("could not process incorporated result: %w", err)
	}

	// process pending approvals only if it's a new collector
	// pending approvals are those we haven't received its result yet,
	// once we received a result and created a new collector, we find the pending
	// approvals for this result, and process them
	// newIncorporatedResult should be true only for one goroutine even if multiple access this code at the same
	// time, ensuring that processing of pending approvals happens once for particular assignment
	if lazyCollector.Created {
		err = c.processPendingApprovals(lazyCollector.Collector)
		if err != nil {
			return fmt.Errorf("could not process cached approvals:  %w", err)
		}
	}

	return nil
}

func (c *Core) ProcessIncorporatedResult(result *flow.IncorporatedResult) error {
	err := c.processIncorporatedResult(result)

	// we expect that only engine.UnverifiableInputError,
	// engine.OutdatedInputError, engine.InvalidInputError are expected, otherwise it's an exception
	if engine.IsUnverifiableInputError(err) || engine.IsOutdatedInputError(err) || engine.IsInvalidInputError(err) {
		logger := c.log.Info()
		if engine.IsInvalidInputError(err) {
			logger = c.log.Error()
		}

		logger.Err(err).Msgf("could not process incorporated result %v", result.ID())
		return nil
	}

	return err
}

// checkBlockOutdated performs a sanity check if block is outdated
// Returns:
// * engine.UnverifiableInputError - sentinel error in case we haven't discovered requested blockID
// * engine.OutdatedInputError - sentinel error in case block is outdated
// * exception in case of unknown internal error
// * nil - block isn't sealed
func (c *Core) checkBlockOutdated(blockID flow.Identifier) error {
	block, err := c.headers.ByBlockID(blockID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return fmt.Errorf("failed to retrieve header for block %x: %w", blockID, err)
		}
		return engine.NewUnverifiableInputError("no header for block: %v", blockID)
	}

	// it's important to use atomic operation to make sure that we have correct ordering
	lastSealedHeight := c.lastSealedHeight()
	// drop approval, if it is for block whose height is lower or equal to already sealed height
	if lastSealedHeight >= block.Height {
		return engine.NewOutdatedInputErrorf("requested processing for already sealed block height")
	}

	return nil
}

func (c *Core) ProcessApproval(approval *flow.ResultApproval) error {
	startTime := time.Now()
	approvalSpan := c.tracer.StartSpan(approval.ID(), trace.CONMatchOnApproval)

	err := c.processApproval(approval)

	c.metrics.OnApprovalProcessingDuration(time.Since(startTime))
	approvalSpan.Finish()

	// we expect that only engine.UnverifiableInputError,
	// engine.OutdatedInputError, engine.InvalidInputError are expected, otherwise it's an exception
	if engine.IsUnverifiableInputError(err) || engine.IsOutdatedInputError(err) || engine.IsInvalidInputError(err) {
		logger := c.log.Info()
		if engine.IsInvalidInputError(err) {
			logger = c.log.Error()
		}

		logger.Err(err).
			Hex("approval_id", logging.Entity(approval)).
			Msgf("could not process result approval")

		return nil
	}
	marshalled, err := json.Marshal(approval)
	if err != nil {
		marshalled = []byte("json_marshalling_failed")
	}
	c.log.Error().Err(err).
		Hex("approval_id", logging.Entity(approval)).
		Str("approval", string(marshalled)).
		Msgf("unexpected error processing result approval")

	return fmt.Errorf("internal error processing result approval %x: %w", approval.ID(), err)

}

// processApproval implements business logic for processing single approval
// Returns:
// * engine.InvalidInputError - result approval is invalid
// * engine.UnverifiableInputError - result approval is unverifiable since referenced block cannot be found
// * engine.OutdatedInputError - result approval is outdated for instance block was already sealed
// * exception in case of any other error, usually this is not expected
// * nil - successfully processed result approval
func (c *Core) processApproval(approval *flow.ResultApproval) error {
	err := c.checkBlockOutdated(approval.Body.BlockID)
	if err != nil {
		return fmt.Errorf("won't process approval for oudated block (%x): %w", approval.Body.BlockID, err)
	}

	if collector, processable := c.collectorTree.GetCollector(approval.Body.ExecutionResultID); collector != nil {
		if !processable {
			return engine.NewOutdatedInputErrorf("collector for %s is marked as non processable", approval.Body.ExecutionResultID)
		}

		// if there is a collector it means that we have received execution result and we are ready
		// to process approvals
		err = collector.ProcessApproval(approval)
		if err != nil {
			return fmt.Errorf("could not process assignment: %w", err)
		}
	} else {
		// in case we haven't received execution result, cache it and process later.
		c.approvalsCache.Put(approval)
	}

	return nil
}

func (c *Core) checkEmergencySealing(lastSealedHeight, lastFinalizedHeight uint64) error {
	if !c.options.emergencySealingActive {
		return nil
	}

	emergencySealingHeight := lastSealedHeight + approvals.DefaultEmergencySealingThreshold

	// we are interested in all collectors that match condition:
	// lastSealedBlock + sealing.DefaultEmergencySealingThreshold < lastFinalizedHeight
	// in other words we should check for emergency sealing only if threshold was reached
	if emergencySealingHeight >= lastFinalizedHeight {
		return nil
	}

	delta := lastFinalizedHeight - emergencySealingHeight
	// if block is emergency sealable depends on it's incorporated block height
	// collectors tree stores collector by executed block height
	// we need to select multiple levels to find eligible collectors for emergency sealing
	for _, collector := range c.collectorTree.GetCollectorsByInterval(lastSealedHeight, lastSealedHeight+delta) {
		err := collector.CheckEmergencySealing(lastFinalizedHeight)
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) processPendingApprovals(collector *approvals.AssignmentCollector) error {
	// filter cached approvals for concrete execution result
	for _, approval := range c.approvalsCache.TakeByResultID(collector.ResultID) {
		err := collector.ProcessApproval(approval)
		if err != nil {
			if engine.IsInvalidInputError(err) {
				c.log.Debug().
					Hex("result_id", collector.ResultID[:]).
					Err(err).
					Msgf("invalid approval with id %s", approval.ID())
			} else {
				return fmt.Errorf("could not process assignment: %w", err)
			}
		}
	}

	return nil
}

func (c *Core) ProcessFinalizedBlock(finalizedBlockID flow.Identifier) error {
	finalized, err := c.headers.ByBlockID(finalizedBlockID)
	if err != nil {
		return fmt.Errorf("could not retrieve header for finalized block %s", finalizedBlockID)
	}

	// no need to process already finalized blocks
	if finalized.Height <= c.lastFinalizedHeight() {
		return nil
	}

	// it's important to use atomic operation to make sure that we have correct ordering
	atomic.StoreUint64(&c.atomicLastFinalizedHeight, finalized.Height)

	seal, err := c.seals.ByBlockID(finalizedBlockID)
	if err != nil {
		return fmt.Errorf("could not retrieve seal for finalized block %s", finalizedBlockID)
	}
	lastSealed, err := c.headers.ByBlockID(seal.BlockID)
	if err != nil {
		c.log.Fatal().Err(err).Msgf("could not retrieve last sealed block %s", seal.BlockID)
	}

	// it's important to use atomic operation to make sure that we have correct ordering
	atomic.StoreUint64(&c.atomicLastSealedHeight, lastSealed.Height)

	// check if there are stale results qualified for emergency sealing
	err = c.checkEmergencySealing(lastSealed.Height, finalized.Height)
	if err != nil {
		return fmt.Errorf("could not check emergency sealing at block %v", finalizedBlockID)
	}

	// finalize forks to stop collecting approvals for orphan collectors
	c.collectorTree.FinalizeForkAtLevel(finalized, lastSealed)

	// as soon as we discover new sealed height, proceed with pruning collectors
	pruned, err := c.collectorTree.PruneUpToHeight(lastSealed.Height)
	if err != nil {
		return fmt.Errorf("could not prune collectorTree tree at block %v", finalizedBlockID)
	}

	// remove all pending items that we might have requested
	c.requestTracker.Remove(pruned...)

	err = c.requestPendingApprovals(lastSealed.Height, finalized.Height)
	if err != nil {
		return fmt.Errorf("internal error while requesting pending approvals: %w", err)
	}

	return nil
}

// requestPendingApprovals requests approvals for chunks that haven't collected
// enough approvals. When the number of unsealed finalized blocks exceeds the
// threshold, we go through the entire mempool of incorporated-results, which
// haven't yet been sealed, and check which chunks need more approvals. We only
// request approvals if the block incorporating the result is below the
// threshold.
//
//                                   threshold
//                              |                   |
// ... <-- A <-- A+1 <- ... <-- D <-- D+1 <- ... -- F
//       sealed       maxHeightForRequesting      final
func (c *Core) requestPendingApprovals(lastSealedHeight, lastFinalizedHeight uint64) error {
	// skip requesting approvals if they are not required for sealing
	if c.options.requiredApprovalsForSealConstruction == 0 {
		return nil
	}

	if lastSealedHeight+c.options.approvalRequestsThreshold >= lastFinalizedHeight {
		return nil
	}

	// Reaching the following code implies:
	// 0 <= sealed.Height < final.Height - approvalRequestsThreshold
	// Hence, the following operation cannot underflow
	maxHeightForRequesting := lastFinalizedHeight - c.options.approvalRequestsThreshold

	for _, collector := range c.collectorTree.GetCollectorsByInterval(lastSealedHeight, maxHeightForRequesting) {
		err := collector.RequestMissingApprovals(maxHeightForRequesting)
		if err != nil {
			return err
		}
	}

	return nil
}
