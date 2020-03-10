// Code generated by mockery v1.0.0. DO NOT EDIT.

package mock

import (
	"github.com/stretchr/testify/mock"

	"github.com/dapperlabs/flow-go/crypto/random"
	"github.com/dapperlabs/flow-go/model/flow"
	"github.com/dapperlabs/flow-go/module/assignment"
)

// ChunkAssigner is an autogenerated mock type for the ChunkAssigner type
type ChunkAssigner struct {
	mock.Mock
}

// Assigner provides a mock function with given fields: ids, chunks, rng
func (_m *ChunkAssigner) Assigner(ids flow.IdentityList, chunks flow.ChunkList, rng random.RandomGenerator) (*assignment.Assignment, error) {
	ret := _m.Called(ids, chunks, rng)

	var r0 *assignment.Assignment
	if rf, ok := ret.Get(0).(func(flow.IdentityList, flow.ChunkList, random.RandomGenerator) *assignment.Assignment); ok {
		r0 = rf(ids, chunks, rng)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*assignment.Assignment)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(flow.IdentityList, flow.ChunkList, random.RandomGenerator) error); ok {
		r1 = rf(ids, chunks, rng)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
