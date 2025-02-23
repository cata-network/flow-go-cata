package state_test

import (
	"context"
	"testing"

	"github.com/dgraph-io/badger/v2"
	"github.com/golang/mock/gomock"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ledger2 "github.com/onflow/flow-go/ledger"
	"github.com/onflow/flow-go/ledger/common/pathfinder"

	"github.com/onflow/flow-go/engine/execution/state"
	"github.com/onflow/flow-go/engine/execution/state/delta"
	ledger "github.com/onflow/flow-go/ledger/complete"
	"github.com/onflow/flow-go/ledger/complete/wal/fixtures"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/metrics"
	"github.com/onflow/flow-go/module/trace"
	storage "github.com/onflow/flow-go/storage/mock"
	"github.com/onflow/flow-go/storage/mocks"
	"github.com/onflow/flow-go/utils/unittest"
)

func prepareTest(f func(t *testing.T, es state.ExecutionState, l *ledger.Ledger)) func(*testing.T) {
	return func(t *testing.T) {
		unittest.RunWithBadgerDB(t, func(badgerDB *badger.DB) {
			metricsCollector := &metrics.NoopCollector{}
			diskWal := &fixtures.NoopWAL{}
			ls, err := ledger.NewLedger(diskWal, 100, metricsCollector, zerolog.Nop(), ledger.DefaultPathFinderVersion)
			require.NoError(t, err)
			compactor := fixtures.NewNoopCompactor(ls)
			<-compactor.Ready()
			defer func() {
				<-ls.Done()
				<-compactor.Done()
			}()

			ctrl := gomock.NewController(t)

			stateCommitments := mocks.NewMockCommits(ctrl)
			blocks := mocks.NewMockBlocks(ctrl)
			headers := mocks.NewMockHeaders(ctrl)
			collections := mocks.NewMockCollections(ctrl)
			events := mocks.NewMockEvents(ctrl)
			serviceEvents := mocks.NewMockServiceEvents(ctrl)
			txResults := mocks.NewMockTransactionResults(ctrl)

			stateCommitment := ls.InitialState()

			stateCommitments.EXPECT().ByBlockID(gomock.Any()).Return(flow.StateCommitment(stateCommitment), nil)

			chunkDataPacks := new(storage.ChunkDataPacks)

			results := new(storage.ExecutionResults)
			myReceipts := new(storage.MyExecutionReceipts)

			es := state.NewExecutionState(
				ls, stateCommitments, blocks, headers, collections, chunkDataPacks, results, myReceipts, events, serviceEvents, txResults, badgerDB, trace.NewNoopTracer(),
			)

			f(t, es, ls)
		})
	}
}

func TestExecutionStateWithTrieStorage(t *testing.T) {
	registerID1 := flow.NewRegisterID("fruit", "")

	registerID2 := flow.NewRegisterID("vegetable", "")

	t.Run("commit write and read new state", prepareTest(func(t *testing.T, es state.ExecutionState, l *ledger.Ledger) {
		// TODO: use real block ID
		sc1, err := es.StateCommitmentByBlockID(context.Background(), flow.Identifier{})
		assert.NoError(t, err)

		view1 := delta.NewDeltaView(es.NewStorageSnapshot(sc1))

		err = view1.Set(registerID1, flow.RegisterValue("apple"))
		assert.NoError(t, err)
		err = view1.Set(registerID2, flow.RegisterValue("carrot"))
		assert.NoError(t, err)

		sc2, update, err := state.CommitDelta(l, view1.Delta(), sc1)
		assert.NoError(t, err)

		assert.Equal(t, sc1[:], update.RootHash[:])
		assert.Len(t, update.Paths, 2)
		assert.Len(t, update.Payloads, 2)

		key1 := ledger2.NewKey(
			[]ledger2.KeyPart{
				ledger2.NewKeyPart(0, []byte(registerID1.Owner)),
				ledger2.NewKeyPart(2, []byte(registerID1.Key)),
			})
		path1, err := pathfinder.KeyToPath(key1, ledger.DefaultPathFinderVersion)
		assert.NoError(t, err)

		key2 := ledger2.NewKey(
			[]ledger2.KeyPart{
				ledger2.NewKeyPart(0, []byte(registerID2.Owner)),
				ledger2.NewKeyPart(2, []byte(registerID2.Key)),
			})
		path2, err := pathfinder.KeyToPath(key2, ledger.DefaultPathFinderVersion)
		assert.NoError(t, err)

		assert.Equal(t, path1, update.Paths[0])
		assert.Equal(t, path2, update.Paths[1])

		k1, err := update.Payloads[0].Key()
		require.NoError(t, err)

		k2, err := update.Payloads[1].Key()
		require.NoError(t, err)

		assert.Equal(t, key1, k1)
		assert.Equal(t, key2, k2)

		assert.Equal(t, []byte("apple"), []byte(update.Payloads[0].Value()))
		assert.Equal(t, []byte("carrot"), []byte(update.Payloads[1].Value()))

		view2 := delta.NewDeltaView(es.NewStorageSnapshot(sc2))

		b1, err := view2.Get(registerID1)
		assert.NoError(t, err)
		b2, err := view2.Get(registerID2)
		assert.NoError(t, err)

		assert.Equal(t, flow.RegisterValue("apple"), b1)
		assert.Equal(t, flow.RegisterValue("carrot"), b2)
	}))

	t.Run("commit write and read previous state", prepareTest(func(t *testing.T, es state.ExecutionState, l *ledger.Ledger) {
		// TODO: use real block ID
		sc1, err := es.StateCommitmentByBlockID(context.Background(), flow.Identifier{})
		assert.NoError(t, err)

		view1 := delta.NewDeltaView(es.NewStorageSnapshot(sc1))

		err = view1.Set(registerID1, []byte("apple"))
		assert.NoError(t, err)
		sc2, _, err := state.CommitDelta(l, view1.Delta(), sc1)
		assert.NoError(t, err)

		// update value and get resulting state commitment
		view2 := delta.NewDeltaView(es.NewStorageSnapshot(sc2))
		err = view2.Set(registerID1, []byte("orange"))
		assert.NoError(t, err)

		sc3, _, err := state.CommitDelta(l, view2.Delta(), sc2)
		assert.NoError(t, err)

		// create a view for previous state version
		view3 := delta.NewDeltaView(es.NewStorageSnapshot(sc2))

		// create a view for new state version
		view4 := delta.NewDeltaView(es.NewStorageSnapshot(sc3))

		// fetch the value at both versions
		b1, err := view3.Get(registerID1)
		assert.NoError(t, err)

		b2, err := view4.Get(registerID1)
		assert.NoError(t, err)

		assert.Equal(t, flow.RegisterValue("apple"), b1)
		assert.Equal(t, flow.RegisterValue("orange"), b2)
	}))

	t.Run("commit delta and read new state", prepareTest(func(t *testing.T, es state.ExecutionState, l *ledger.Ledger) {
		// TODO: use real block ID
		sc1, err := es.StateCommitmentByBlockID(context.Background(), flow.Identifier{})
		assert.NoError(t, err)

		// set initial value
		view1 := delta.NewDeltaView(es.NewStorageSnapshot(sc1))
		err = view1.Set(registerID1, []byte("apple"))
		assert.NoError(t, err)
		err = view1.Set(registerID2, []byte("apple"))
		assert.NoError(t, err)

		sc2, _, err := state.CommitDelta(l, view1.Delta(), sc1)
		assert.NoError(t, err)

		// update value and get resulting state commitment
		view2 := delta.NewDeltaView(es.NewStorageSnapshot(sc2))
		err = view2.Set(registerID1, nil)
		assert.NoError(t, err)

		sc3, _, err := state.CommitDelta(l, view2.Delta(), sc2)
		assert.NoError(t, err)

		// create a view for previous state version
		view3 := delta.NewDeltaView(es.NewStorageSnapshot(sc2))

		// create a view for new state version
		view4 := delta.NewDeltaView(es.NewStorageSnapshot(sc3))

		// fetch the value at both versions
		b1, err := view3.Get(registerID1)
		assert.NoError(t, err)

		b2, err := view4.Get(registerID1)
		assert.NoError(t, err)

		assert.Equal(t, flow.RegisterValue("apple"), b1)
		assert.Empty(t, b2)
	}))

	t.Run("commit delta and persist state commit for the second time should be OK", prepareTest(func(t *testing.T, es state.ExecutionState, l *ledger.Ledger) {
		// TODO: use real block ID
		sc1, err := es.StateCommitmentByBlockID(context.Background(), flow.Identifier{})
		assert.NoError(t, err)

		// set initial value
		view1 := delta.NewDeltaView(es.NewStorageSnapshot(sc1))
		err = view1.Set(registerID1, flow.RegisterValue("apple"))
		assert.NoError(t, err)
		err = view1.Set(registerID2, flow.RegisterValue("apple"))
		assert.NoError(t, err)

		sc2, _, err := state.CommitDelta(l, view1.Delta(), sc1)
		assert.NoError(t, err)

		// committing for the second time should be OK
		sc2Same, _, err := state.CommitDelta(l, view1.Delta(), sc1)
		assert.NoError(t, err)

		require.Equal(t, sc2, sc2Same)
	}))

}
