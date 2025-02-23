// Code generated by mockery v2.21.4. DO NOT EDIT.

package mock

import (
	flow "github.com/onflow/flow-go/model/flow"
	mock "github.com/stretchr/testify/mock"
)

// OnEquivocation is an autogenerated mock type for the OnEquivocation type
type OnEquivocation struct {
	mock.Mock
}

// Execute provides a mock function with given fields: first, other
func (_m *OnEquivocation) Execute(first *flow.Block, other *flow.Block) {
	_m.Called(first, other)
}

type mockConstructorTestingTNewOnEquivocation interface {
	mock.TestingT
	Cleanup(func())
}

// NewOnEquivocation creates a new instance of OnEquivocation. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewOnEquivocation(t mockConstructorTestingTNewOnEquivocation) *OnEquivocation {
	mock := &OnEquivocation{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
