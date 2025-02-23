// Code generated by mockery v2.21.4. DO NOT EDIT.

package mempool

import (
	flow "github.com/onflow/flow-go/model/flow"

	messages "github.com/onflow/flow-go/model/messages"

	mock "github.com/stretchr/testify/mock"
)

// Deltas is an autogenerated mock type for the Deltas type
type Deltas struct {
	mock.Mock
}

// Add provides a mock function with given fields: delta
func (_m *Deltas) Add(delta *messages.ExecutionStateDelta) bool {
	ret := _m.Called(delta)

	var r0 bool
	if rf, ok := ret.Get(0).(func(*messages.ExecutionStateDelta) bool); ok {
		r0 = rf(delta)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// All provides a mock function with given fields:
func (_m *Deltas) All() []*messages.ExecutionStateDelta {
	ret := _m.Called()

	var r0 []*messages.ExecutionStateDelta
	if rf, ok := ret.Get(0).(func() []*messages.ExecutionStateDelta); ok {
		r0 = rf()
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]*messages.ExecutionStateDelta)
		}
	}

	return r0
}

// ByBlockID provides a mock function with given fields: blockID
func (_m *Deltas) ByBlockID(blockID flow.Identifier) (*messages.ExecutionStateDelta, bool) {
	ret := _m.Called(blockID)

	var r0 *messages.ExecutionStateDelta
	var r1 bool
	if rf, ok := ret.Get(0).(func(flow.Identifier) (*messages.ExecutionStateDelta, bool)); ok {
		return rf(blockID)
	}
	if rf, ok := ret.Get(0).(func(flow.Identifier) *messages.ExecutionStateDelta); ok {
		r0 = rf(blockID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*messages.ExecutionStateDelta)
		}
	}

	if rf, ok := ret.Get(1).(func(flow.Identifier) bool); ok {
		r1 = rf(blockID)
	} else {
		r1 = ret.Get(1).(bool)
	}

	return r0, r1
}

// Has provides a mock function with given fields: blockID
func (_m *Deltas) Has(blockID flow.Identifier) bool {
	ret := _m.Called(blockID)

	var r0 bool
	if rf, ok := ret.Get(0).(func(flow.Identifier) bool); ok {
		r0 = rf(blockID)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Limit provides a mock function with given fields:
func (_m *Deltas) Limit() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

// Remove provides a mock function with given fields: blockID
func (_m *Deltas) Remove(blockID flow.Identifier) bool {
	ret := _m.Called(blockID)

	var r0 bool
	if rf, ok := ret.Get(0).(func(flow.Identifier) bool); ok {
		r0 = rf(blockID)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Size provides a mock function with given fields:
func (_m *Deltas) Size() uint {
	ret := _m.Called()

	var r0 uint
	if rf, ok := ret.Get(0).(func() uint); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint)
	}

	return r0
}

type mockConstructorTestingTNewDeltas interface {
	mock.TestingT
	Cleanup(func())
}

// NewDeltas creates a new instance of Deltas. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewDeltas(t mockConstructorTestingTNewDeltas) *Deltas {
	mock := &Deltas{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
