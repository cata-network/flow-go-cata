// (c) 2019 Dapper Labs - ALL RIGHTS RESERVED

package operation

import (
	"github.com/dgraph-io/badger/v2"

	"github.com/onflow/flow-go/model/flow"
)

func InsertHeader(headerID flow.Identifier, header *flow.Header) func(*badger.Txn) error {
	return insert(makePrefix(codeHeader, headerID), header)
}

func RetrieveHeader(blockID flow.Identifier, header *flow.Header) func(*badger.Txn) error {
	return retrieve(makePrefix(codeHeader, blockID), header)
}

// IndexBlockHeight indexes the height of a block. It should only be called on
// finalized blocks.
func IndexBlockHeight(height uint64, blockID flow.Identifier) func(*badger.Txn) error {
	return insert(makePrefix(codeHeightToBlock, height), blockID)
}

// LookupBlockHeight retrieves finalized blocks by height.
func LookupBlockHeight(height uint64, blockID *flow.Identifier) func(*badger.Txn) error {
	return retrieve(makePrefix(codeHeightToBlock, height), blockID)
}

func InsertExecutedBlock(blockID flow.Identifier) func(*badger.Txn) error {
	return insert(makePrefix(codeExecutedBlock), blockID)
}

func UpdateExecutedBlock(blockID flow.Identifier) func(*badger.Txn) error {
	return update(makePrefix(codeExecutedBlock), blockID)
}

func RetrieveExecutedBlock(blockID *flow.Identifier) func(*badger.Txn) error {
	return retrieve(makePrefix(codeExecutedBlock), blockID)
}

// IndexCollectionBlock indexes a block by a collection within that block.
func IndexCollectionBlock(collID flow.Identifier, blockID flow.Identifier) func(*badger.Txn) error {
	return insert(makePrefix(codeCollectionBlock, collID), blockID)
}

func IndexBlockIDByChunkID(chunkID, blockID flow.Identifier) func(*badger.Txn) error {
	return insert(makePrefix(codeIndexBlockByChunkID, chunkID), blockID)
}

// BatchIndexBlockByChunkID indexes blockID by chunkID into a batch
func BatchIndexBlockByChunkID(blockID, chunkID flow.Identifier) func(batch *badger.WriteBatch) error {
	return batchWrite(makePrefix(codeIndexBlockByChunkID, chunkID), blockID)
}

// LookupCollectionBlock looks up a block by a collection within that block.
func LookupCollectionBlock(collID flow.Identifier, blockID *flow.Identifier) func(*badger.Txn) error {
	return retrieve(makePrefix(codeCollectionBlock, collID), blockID)
}

// LookupBlockIDByChunkID looks up a block by a collection within that block.
func LookupBlockIDByChunkID(chunkID flow.Identifier, blockID *flow.Identifier) func(*badger.Txn) error {
	return retrieve(makePrefix(codeIndexBlockByChunkID, chunkID), blockID)
}

// RemoveBlockIDByChunkID removes chunkID-blockID index by chunkID
func RemoveBlockIDByChunkID(chunkID flow.Identifier) func(*badger.Txn) error {
	return remove(makePrefix(codeIndexBlockByChunkID, chunkID))
}

// BatchRemoveBlockIDByChunkID removes chunkID-to-blockID index entries keyed by a chunkID in a provided batch.
// No errors are expected during normal operation, even if no entries are matched.
// If Badger unexpectedly fails to process the request, the error is wrapped in a generic error and returned.
func BatchRemoveBlockIDByChunkID(chunkID flow.Identifier) func(batch *badger.WriteBatch) error {
	return batchRemove(makePrefix(codeIndexBlockByChunkID, chunkID))
}

// FindHeaders iterates through all headers, calling `filter` on each, and adding
// them to the `found` slice if `filter` returned true
func FindHeaders(filter func(header *flow.Header) bool, found *[]flow.Header) func(*badger.Txn) error {
	return traverse(makePrefix(codeHeader), func() (checkFunc, createFunc, handleFunc) {
		check := func(key []byte) bool {
			return true
		}
		var val flow.Header
		create := func() interface{} {
			return &val
		}
		handle := func() error {
			if filter(&val) {
				*found = append(*found, val)
			}
			return nil
		}
		return check, create, handle
	})
}
