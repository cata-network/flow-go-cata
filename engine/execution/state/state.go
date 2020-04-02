package state

import (
	"fmt"

	"github.com/dgraph-io/badger/v2"

	"github.com/dapperlabs/flow-go/engine/execution/state/delta"
	"github.com/dapperlabs/flow-go/model/messages"

	"github.com/dapperlabs/flow-go/model/flow"
	"github.com/dapperlabs/flow-go/storage"
	"github.com/dapperlabs/flow-go/storage/badger/operation"
	"github.com/dapperlabs/flow-go/storage/badger/procedure"
)

// ReadOnlyExecutionState allows to read the execution state
type ReadOnlyExecutionState interface {
	// NewView creates a new ready-only view at the given state commitment.
	NewView(flow.StateCommitment) *delta.View

	GetRegisters(flow.StateCommitment, []flow.RegisterID) ([]flow.RegisterValue, error)
	GetChunkRegisters(flow.Identifier) (flow.Ledger, error)

	GetRegistersWithProofs(flow.StateCommitment, []flow.RegisterID) ([]flow.RegisterValue, []flow.StorageProof, error)

	// StateCommitmentByBlockID returns the final state commitment for the provided block ID.
	StateCommitmentByBlockID(flow.Identifier) (flow.StateCommitment, error)

	// ChunkHeaderByChunkID returns the chunk header for the provided chunk ID.
	ChunkHeaderByChunkID(flow.Identifier) (*flow.ChunkHeader, error)

	// ChunkHeaderByChunkID retrieve a chunk data pack given the chunk ID.
	ChunkDataPackByChunkID(flow.Identifier) (*flow.ChunkDataPack, error)

	GetExecutionResultID(blockID flow.Identifier) (flow.Identifier, error)

	FindLatestFinalizedAndExecutedBlock() (*flow.Header, error)

	RetrieveStateDelta(blockID flow.Identifier) (*messages.ExecutionStateDelta, error)
}

// TODO Many operations here are should be transactional, so we need to refactor this
// to store a reference to DB and compose operations and procedures rather then
// just being amalgamate of proxies for single transactions operation

// ExecutionState is an interface used to access and mutate the execution state of the blockchain.
type ExecutionState interface {
	ReadOnlyExecutionState

	// CommitDelta commits a register delta and returns the new state commitment.
	CommitDelta(delta.Delta, flow.StateCommitment) (flow.StateCommitment, error)

	// PersistStateCommitment saves a state commitment by the given block ID.
	PersistStateCommitment(flow.Identifier, flow.StateCommitment) error

	// PersistChunkHeader saves a chunk header by chunk ID.
	PersistChunkHeader(*flow.ChunkHeader) error

	// PersistChunkDataPack stores a chunk data pack by chunk ID.
	PersistChunkDataPack(*flow.ChunkDataPack) error

	PersistExecutionResult(blockID flow.Identifier, result flow.ExecutionResult) error

	PersistStateViews(blockID flow.Identifier, views []*delta.View) error
}

type state struct {
	ls               storage.Ledger
	commits          storage.Commits
	chunkHeaders     storage.ChunkHeaders
	chunkDataPacks   storage.ChunkDataPacks
	executionResults storage.ExecutionResults
	db               *badger.DB
}

// NewExecutionState returns a new execution state access layer for the given ledger storage.
func NewExecutionState(
	ls storage.Ledger,
	commits storage.Commits,
	chunkHeaders storage.ChunkHeaders,
	chunkDataPacks storage.ChunkDataPacks,
	executionResult storage.ExecutionResults,
	db *badger.DB,
) ExecutionState {
	return &state{
		ls:               ls,
		commits:          commits,
		chunkHeaders:     chunkHeaders,
		chunkDataPacks:   chunkDataPacks,
		executionResults: executionResult,
		db:               db,
	}
}

func LedgerGetRegister(ledger storage.Ledger, commitment flow.StateCommitment) delta.GetRegisterFunc {
	return func(key flow.RegisterID) ([]byte, error) {
		values, err := ledger.GetRegisters(
			[]flow.RegisterID{key},
			commitment,
		)
		if err != nil {
			return nil, fmt.Errorf("error getting register (%s) value at %x: %w", key, commitment, err)
		}

		if len(values) == 0 {
			return nil, nil
		}

		return values[0], nil
	}
}

func (s *state) NewView(commitment flow.StateCommitment) *delta.View {
	return delta.NewView(LedgerGetRegister(s.ls, commitment))
}

func CommitDelta(ledger storage.Ledger, delta delta.Delta, baseState flow.StateCommitment) (flow.StateCommitment, error) {
	ids, values := delta.RegisterUpdates()

	// TODO: update CommitDelta to also return proofs
	commit, _, err := ledger.UpdateRegistersWithProof(ids, values, baseState)
	if err != nil {
		return nil, err
	}

	return commit, nil
}

func (s *state) CommitDelta(delta delta.Delta, baseState flow.StateCommitment) (flow.StateCommitment, error) {
	return CommitDelta(s.ls, delta, baseState)
}

func (s *state) GetRegisters(
	commit flow.StateCommitment,
	registerIDs []flow.RegisterID,
) ([]flow.RegisterValue, error) {
	return s.ls.GetRegisters(registerIDs, commit)
}

func (s *state) GetRegistersWithProofs(
	commit flow.StateCommitment,
	registerIDs []flow.RegisterID,
) ([]flow.RegisterValue, []flow.StorageProof, error) {
	return s.ls.GetRegistersWithProof(registerIDs, commit)
}

func (s *state) GetChunkRegisters(chunkID flow.Identifier) (flow.Ledger, error) {
	chunkHeader, err := s.ChunkHeaderByChunkID(chunkID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunk header: %w", err)
	}

	registerIDs := chunkHeader.RegisterIDs

	registerValues, err := s.GetRegisters(chunkHeader.StartState, registerIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve chunk register values: %w", err)
	}

	l := make(flow.Ledger)

	for i, registerID := range registerIDs {
		l[string(registerID)] = registerValues[i]
	}

	return l, nil
}

func (s *state) StateCommitmentByBlockID(blockID flow.Identifier) (flow.StateCommitment, error) {
	return s.commits.ByID(blockID)
}

func (s *state) PersistStateCommitment(blockID flow.Identifier, commit flow.StateCommitment) error {
	return s.commits.Store(blockID, commit)
}

func (s *state) ChunkHeaderByChunkID(chunkID flow.Identifier) (*flow.ChunkHeader, error) {
	return s.chunkHeaders.ByID(chunkID)
}

func (s *state) PersistChunkHeader(c *flow.ChunkHeader) error {
	return s.chunkHeaders.Store(c)
}

func (s *state) ChunkDataPackByChunkID(chunkID flow.Identifier) (*flow.ChunkDataPack, error) {
	return s.chunkDataPacks.ByChunkID(chunkID)
}

func (s *state) PersistChunkDataPack(c *flow.ChunkDataPack) error {
	return s.chunkDataPacks.Store(c)
}

func (s *state) GetExecutionResultID(blockID flow.Identifier) (flow.Identifier, error) {
	return s.executionResults.Lookup(blockID)
}

func (s *state) PersistExecutionResult(blockID flow.Identifier, result flow.ExecutionResult) error {
	err := s.executionResults.Store(&result)
	if err != nil {
		return fmt.Errorf("could not persist execution result: %w", err)
	}
	// TODO if the second operation fails we should remove stored execution result
	// This is global execution storage problem - see TODO at the top
	return s.executionResults.Index(blockID, result.ID())
}

// FindLatestFinalizedAndExecutedBlock finds latest block which is both finalized
// and we have state commitment for it
func (s *state) FindLatestFinalizedAndExecutedBlock() (*flow.Header, error) {
	var header flow.Header
	err := s.db.View(func(txn *badger.Txn) error {
		err := procedure.RetrieveLatestFinalizedHeader(&header)(txn)
		if err != nil {
			return fmt.Errorf("cannot find latest finalized block: %w", err)
		}

		var stateCommitment flow.StateCommitment
		err = operation.LookupStateCommitment(header.ID(), &stateCommitment)(txn)

		for err == storage.ErrNotFound {
			parentID := header.ParentID
			err = operation.RetrieveHeader(parentID, &header)(txn)
			if err != nil {
				return fmt.Errorf("cannot retrieve block (%s): %w", header.ID(), err)
			}
			err = operation.LookupStateCommitment(header.ID(), &stateCommitment)(txn)
		}
		if err != nil {
			return fmt.Errorf("error while finding latest finalized block: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &header, nil
}

func (s *state) PersistStateViews(blockID flow.Identifier, views []*delta.View) error {
	return s.db.Update(func(txn *badger.Txn) error {
		return operation.InsertExecutionStateViews(blockID, views)(txn)
	})
}

func (s *state) RetrieveStateDelta(blockID flow.Identifier) (*messages.ExecutionStateDelta, error) {
	var block flow.Block
	var startStateCommitment flow.StateCommitment
	var endStateCommitment flow.StateCommitment
	var stateViews []*delta.View
	err := s.db.View(func(txn *badger.Txn) error {
		err := procedure.RetrieveBlock(blockID, &block)(txn)
		if err != nil {
			return err
		}
		err = operation.LookupStateCommitment(blockID, &endStateCommitment)(txn)
		if err != nil {
			return err
		}
		err = operation.LookupStateCommitment(block.ParentID, &startStateCommitment)(txn)
		if err != nil {
			return err
		}

		return operation.RetrieveExecutionStateViews(blockID, stateViews)(txn)
	})
	if err != nil {
		return nil, err
	}
	return &messages.ExecutionStateDelta{
		Block:      &block,
		StateViews: stateViews,
		StartState: startStateCommitment,
		EndState:   endStateCommitment,
	}, nil
}
