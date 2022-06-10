package tracker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/dgraph-io/badger/v2"
	"github.com/hashicorp/go-multierror"
	"github.com/ipfs/go-cid"
	"github.com/rs/zerolog"

	"github.com/onflow/flow-go/module/blobs"
)

// badger key prefixes
const (
	prefixGlobalState  byte = iota + 1 // global state variables
	prefixLatestHeight                 // tracks, for each blob, the latest height at which there exists a block whose execution data contains the blob
	prefixBlobRecord                   // tracks the set of blobs at each height
)

const (
	globalStateFulfilledHeight = iota + 1 // latest fulfilled block height
	globalStatePrunedHeight               // latest pruned block height
)

func retryOnConflict(db *badger.DB, fn func(txn *badger.Txn) error) error {
	for {
		err := db.Update(fn)
		if errors.Is(err, badger.ErrConflict) {
			continue
		}
		return err
	}
}

const globalStateKeyLength = 2

func makeGlobalStateKey(state byte) []byte {
	globalStateKey := make([]byte, globalStateKeyLength)
	globalStateKey[0] = prefixGlobalState
	globalStateKey[1] = state
	return globalStateKey
}

const blobRecordKeyLength = 1 + 8 + blobs.CidLength

func makeBlobRecordKey(blockHeight uint64, c cid.Cid) []byte {
	blobRecordKey := make([]byte, blobRecordKeyLength)
	blobRecordKey[0] = prefixBlobRecord
	binary.LittleEndian.PutUint64(blobRecordKey[1:], blockHeight)
	copy(blobRecordKey[1+8:], c.Bytes())
	return blobRecordKey
}

func parseBlobRecordKey(key []byte) (uint64, cid.Cid, error) {
	blockHeight := binary.LittleEndian.Uint64(key[1:])
	c, err := cid.Cast(key[1+8:])
	return blockHeight, c, err
}

const latestHeightKeyLength = 1 + blobs.CidLength

func makeLatestHeightKey(c cid.Cid) []byte {
	latestHeightKey := make([]byte, latestHeightKeyLength)
	latestHeightKey[0] = prefixLatestHeight
	copy(latestHeightKey[1:], c.Bytes())
	return latestHeightKey
}

// getBatchItemCountLimit returns the maximum number of items that can be included in a single batch
// transaction based on the number / total size of updates per item.
func getBatchItemCountLimit(db *badger.DB, writeCountPerItem int64, writeSizePerItem int64) int {
	totalSizePerItem := 2*writeCountPerItem + writeSizePerItem // 2 bytes per entry for user and internal meta
	maxItemCountByWriteCount := db.MaxBatchCount() / writeCountPerItem
	maxItemCountByWriteSize := db.MaxBatchSize() / totalSizePerItem

	if maxItemCountByWriteCount < maxItemCountByWriteSize {
		return int(maxItemCountByWriteCount)
	} else {
		return int(maxItemCountByWriteSize)
	}
}

// TrackBlobsFun is passed to the UpdateFn provided to Storage.Update,
// and can be called to track a list of cids at a given block height.
// It returns an error if the update failed.
type TrackBlobsFn func(blockHeight uint64, cids ...cid.Cid) error

// UpdateFn is implemented by the user and passed to Storage.Update,
// which ensures that it will never be run concurrently with any call
// to Storage.Prune.
// Any returned error will be returned from the surrounding call to Storage.Update.
// The function must never make any calls to the Storage interface itself,
// and should instead only modify the storage via the provided TrackBlobsFn.
type UpdateFn func(TrackBlobsFn) error

// PruneCallback is a function which can be provided by the user which
// is called for each CID when the last height at which that CID appears
// is pruned.
// Any returned error will be returned from the surrounding call to Storage.Prune.
// The prune callback can be used to delete the corresponding
// blob data from the blob store.
type PruneCallback func(cid.Cid) error

type Storage interface {
	// Update is used to track new blob CIDs.
	// It can be used to track blobs for both sealed and unsealed
	// heights, and the same blob may be added multiple times for
	// different heights.
	// The same blob may also be added multiple times for the same
	// height, but it will only be tracked once per height.
	Update(UpdateFn) error

	GetFulfilledHeight() (uint64, error)

	// SetFulfilledHeight updates the fulfilled height value,
	// which is the highest block height `h` such that all
	// heights <= `h` are sealed and the sealed execution data
	// has been downloaded.
	// It is up to the caller to ensure that this is never
	// called with a value lower than the pruned height.
	SetFulfilledHeight(height uint64) error

	GetPrunedHeight() (uint64, error)

	// Prune removes all data from storage corresponding to
	// block heights up to and including the given height,
	// and updates the latest pruned height value.
	// It locks the Storage and ensures that no other writes
	// can occur during the pruning.
	// It is up to the caller to ensure that this is never
	// called with a value higher than the fulfilled height.
	Prune(height uint64) error
}

// The storage component tracks the following information:
// * the latest pruned height
// * the latest fulfilled height
// * the set of CIDs of the execution data blobs we know about at each height, so that
//   once we prune a fulfilled height we can remove the blob data from local storage
// * for each CID, the most recent height that it was observed at, so that when pruning
//   a fulfilled height we don't remove any blob data that is still needed at higher heights
// The storage component calls the given prune callback for a CID when the last height
// at which that CID appears is pruned. The prune callback can be used to delete the
// corresponding blob data from the blob store.
type storage struct {
	// ensures that pruning operations are not run concurrently with any other db writes
	// we acquire the read lock when we want to perform a non-prune WRITE
	// we acquire the write lock when we want to perform a prune WRITE
	mu sync.RWMutex

	db            *badger.DB
	pruneCallback PruneCallback
	logger        zerolog.Logger
}

type StorageOption func(*storage)

func WithPruneCallback(callback PruneCallback) StorageOption {
	return func(s *storage) {
		s.pruneCallback = callback
	}
}

func OpenStorage(dbPath string, startHeight uint64, logger zerolog.Logger, opts ...StorageOption) (*storage, error) {
	db, err := badger.Open(badger.LSMOnlyOptions(dbPath))
	if err != nil {
		return nil, fmt.Errorf("could not open tracker db: %w", err)
	}

	storage := &storage{
		db:            db,
		pruneCallback: func(c cid.Cid) error { return nil },
		logger:        logger.With().Str("module", "tracker_storage").Logger(),
	}

	for _, opt := range opts {
		opt(storage)
	}

	if err := storage.init(startHeight); err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	return storage, nil
}

func (s *storage) init(startHeight uint64) error {
	fulfilledHeight, fulfilledHeightErr := s.GetFulfilledHeight()
	prunedHeight, prunedHeightErr := s.GetPrunedHeight()

	if fulfilledHeightErr == nil && prunedHeightErr == nil {
		if prunedHeight > fulfilledHeight {
			return fmt.Errorf("inconsistency detected: pruned height is greater than fulfilled height")
		}

		// replay pruning in case it was interrupted during previous shutdown
		if err := s.Prune(prunedHeight); err != nil {
			return fmt.Errorf("failed to replay pruning: %w", err)
		}
	} else if errors.Is(fulfilledHeightErr, badger.ErrKeyNotFound) && errors.Is(prunedHeightErr, badger.ErrKeyNotFound) {
		// db is empty, we need to bootstrap it
		if err := s.bootstrap(startHeight); err != nil {
			return fmt.Errorf("failed to bootstrap storage: %w", err)
		}
	} else {
		return multierror.Append(fulfilledHeightErr, prunedHeightErr).ErrorOrNil()
	}

	return nil
}

func (s *storage) bootstrap(startHeight uint64) error {
	fulfilledHeightKey := makeGlobalStateKey(globalStateFulfilledHeight)
	fulfilledHeightValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(fulfilledHeightValue, startHeight)

	prunedHeightKey := makeGlobalStateKey(globalStatePrunedHeight)
	prunedHeightValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(prunedHeightValue, startHeight)

	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(fulfilledHeightKey, fulfilledHeightValue); err != nil {
			return fmt.Errorf("failed to set fulfilled height value: %w", err)
		}

		if err := txn.Set(prunedHeightKey, prunedHeightValue); err != nil {
			return fmt.Errorf("failed to set pruned height value: %w", err)
		}

		return nil
	})
}

func (s *storage) Update(f UpdateFn) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return f(s.trackBlobs)
}

func (s *storage) SetFulfilledHeight(height uint64) error {
	fulfilledHeightKey := makeGlobalStateKey(globalStateFulfilledHeight)
	fulfilledHeightValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(fulfilledHeightValue, height)

	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(fulfilledHeightKey, fulfilledHeightValue); err != nil {
			return fmt.Errorf("failed to set fulfilled height value: %w", err)
		}

		return nil
	})
}

func (s *storage) GetFulfilledHeight() (uint64, error) {
	fulfilledHeightKey := makeGlobalStateKey(globalStateFulfilledHeight)
	var fulfilledHeight uint64

	if err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(fulfilledHeightKey)
		if err != nil {
			return fmt.Errorf("failed to find fulfilled height entry: %w", err)
		}

		fulfilledHeightValue, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("failed to retrieve fulfilled height value: %w", err)
		}

		fulfilledHeight = binary.LittleEndian.Uint64(fulfilledHeightValue)

		return nil
	}); err != nil {
		return 0, err
	}

	return fulfilledHeight, nil
}

func (s *storage) trackBlob(txn *badger.Txn, blockHeight uint64, c cid.Cid) error {
	if err := txn.Set(makeBlobRecordKey(blockHeight, c), nil); err != nil {
		return fmt.Errorf("failed to add blob record: %w", err)
	}

	latestHeightKey := makeLatestHeightKey(c)
	item, err := txn.Get(latestHeightKey)
	if err != nil {
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return fmt.Errorf("failed to get latest height: %w", err)
		}
	} else {
		value, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("failed to retrieve latest height value: %w", err)
		}

		// don't update the latest height if there is already a higher block height containing this blob
		latestHeight := binary.LittleEndian.Uint64(value)
		if latestHeight >= blockHeight {
			return nil
		}
	}

	latestHeightValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(latestHeightValue, blockHeight)

	if err := txn.Set(latestHeightKey, latestHeightValue); err != nil {
		return fmt.Errorf("failed to set latest height value: %w", err)
	}

	return nil
}

func (s *storage) trackBlobs(blockHeight uint64, cids ...cid.Cid) error {
	cidsPerBatch := 16
	maxCidsPerBatch := getBatchItemCountLimit(s.db, 2, blobRecordKeyLength+latestHeightKeyLength+8)
	if maxCidsPerBatch < cidsPerBatch {
		cidsPerBatch = maxCidsPerBatch
	}

	for len(cids) > 0 {
		batchSize := cidsPerBatch
		if len(cids) < batchSize {
			batchSize = len(cids)
		}
		batch := cids[:batchSize]

		if err := retryOnConflict(s.db, func(txn *badger.Txn) error {
			for _, c := range batch {
				if err := s.trackBlob(txn, blockHeight, c); err != nil {
					return fmt.Errorf("failed to track blob %s: %w", c.String(), err)
				}
			}

			return nil
		}); err != nil {
			return err
		}

		cids = cids[batchSize:]
	}

	return nil
}

func (s *storage) batchDelete(deleteInfos []*deleteInfo) error {
	return s.db.Update(func(txn *badger.Txn) error {
		for _, dInfo := range deleteInfos {
			if err := txn.Delete(makeBlobRecordKey(dInfo.height, dInfo.cid)); err != nil {
				return fmt.Errorf("failed to delete blob record for Cid %s: %w", dInfo.cid.String(), err)
			}

			if dInfo.deleteLatestHeightRecord {
				if err := txn.Delete(makeLatestHeightKey(dInfo.cid)); err != nil {
					return fmt.Errorf("failed to delete latest height record for Cid %s: %w", dInfo.cid.String(), err)
				}
			}
		}

		return nil
	})
}

func (s *storage) batchDeleteItemLimit() int {
	itemsPerBatch := 256
	maxItemsPerBatch := getBatchItemCountLimit(s.db, 2, blobRecordKeyLength+latestHeightKeyLength)
	if maxItemsPerBatch < itemsPerBatch {
		itemsPerBatch = maxItemsPerBatch
	}
	return itemsPerBatch
}

func (s *storage) Prune(height uint64) error {
	blobRecordPrefix := []byte{prefixBlobRecord}
	itemsPerBatch := s.batchDeleteItemLimit()
	var batch []*deleteInfo

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.setPrunedHeight(height); err != nil {
		return err
	}

	if err := s.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.IteratorOptions{
			PrefetchValues: false,
			Prefix:         blobRecordPrefix,
		})
		defer it.Close()

		for it.Seek(blobRecordPrefix); it.ValidForPrefix(blobRecordPrefix); it.Next() {
			blobRecordItem := it.Item()
			blobRecordKey := blobRecordItem.Key()

			blockHeight, blobCid, err := parseBlobRecordKey(blobRecordKey)
			if err != nil {
				return fmt.Errorf("malformed blob record key %v: %w", blobRecordKey, err)
			}

			if blockHeight > height {
				break
			}

			dInfo := &deleteInfo{
				cid:    blobCid,
				height: blockHeight,
			}

			latestHeightKey := makeLatestHeightKey(blobCid)
			latestHeightItem, err := txn.Get(latestHeightKey)
			if err != nil {
				return fmt.Errorf("failed to get latest height entry for Cid %s: %w", blobCid.String(), err)
			}

			latestHeightValue, err := latestHeightItem.ValueCopy(nil)
			if err != nil {
				return fmt.Errorf("failed to retrieve latest height value for Cid %s: %w", blobCid.String(), err)
			}

			// a blob is only removable if it is not referenced by any blob tree at a higher height
			latestHeight := binary.LittleEndian.Uint64(latestHeightValue)
			if latestHeight < blockHeight {
				// this should never happen
				return fmt.Errorf(
					"inconsistency detected: latest height recorded for Cid %s is %d, but blob record exists at height %d",
					blobCid.String(), latestHeight, blockHeight,
				)
			} else if latestHeight == blockHeight {
				if err := s.pruneCallback(blobCid); err != nil {
					return err
				}
				dInfo.deleteLatestHeightRecord = true
			}

			batch = append(batch, dInfo)
			if len(batch) == itemsPerBatch {
				if err := s.batchDelete(batch); err != nil {
					return err
				}
				batch = nil
			}
		}

		if len(batch) > 0 {
			if err := s.batchDelete(batch); err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return err
	}

	// this is a good time to do garbage collection
	if err := s.db.RunValueLogGC(0.5); err != nil {
		s.logger.Err(err).Msg("failed to run value log garbage collection")
	}

	return nil
}

func (s *storage) setPrunedHeight(height uint64) error {
	prunedHeightKey := makeGlobalStateKey(globalStatePrunedHeight)
	prunedHeightValue := make([]byte, 8)
	binary.LittleEndian.PutUint64(prunedHeightValue, height)

	return s.db.Update(func(txn *badger.Txn) error {
		if err := txn.Set(prunedHeightKey, prunedHeightValue); err != nil {
			return fmt.Errorf("failed to set pruned height value: %w", err)
		}

		return nil
	})
}

func (s *storage) GetPrunedHeight() (uint64, error) {
	prunedHeightKey := makeGlobalStateKey(globalStatePrunedHeight)
	var prunedHeight uint64

	if err := s.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(prunedHeightKey)
		if err != nil {
			return fmt.Errorf("failed to find pruned height entry: %w", err)
		}

		prunedHeightValue, err := item.ValueCopy(nil)
		if err != nil {
			return fmt.Errorf("failed to retrieve pruned height value: %w", err)
		}

		prunedHeight = binary.LittleEndian.Uint64(prunedHeightValue)

		return nil
	}); err != nil {
		return 0, err
	}

	return prunedHeight, nil
}

type deleteInfo struct {
	cid                      cid.Cid
	height                   uint64
	deleteLatestHeightRecord bool
}
