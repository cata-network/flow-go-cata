// Code generated by mockery v2.21.4. DO NOT EDIT.

package mock

import (
	crypto "github.com/onflow/flow-go/crypto"
	flow "github.com/onflow/flow-go/model/flow"

	messages "github.com/onflow/flow-go/model/messages"

	mock "github.com/stretchr/testify/mock"
)

// DKGContractClient is an autogenerated mock type for the DKGContractClient type
type DKGContractClient struct {
	mock.Mock
}

// Broadcast provides a mock function with given fields: msg
func (_m *DKGContractClient) Broadcast(msg messages.BroadcastDKGMessage) error {
	ret := _m.Called(msg)

	var r0 error
	if rf, ok := ret.Get(0).(func(messages.BroadcastDKGMessage) error); ok {
		r0 = rf(msg)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// ReadBroadcast provides a mock function with given fields: fromIndex, referenceBlock
func (_m *DKGContractClient) ReadBroadcast(fromIndex uint, referenceBlock flow.Identifier) ([]messages.BroadcastDKGMessage, error) {
	ret := _m.Called(fromIndex, referenceBlock)

	var r0 []messages.BroadcastDKGMessage
	var r1 error
	if rf, ok := ret.Get(0).(func(uint, flow.Identifier) ([]messages.BroadcastDKGMessage, error)); ok {
		return rf(fromIndex, referenceBlock)
	}
	if rf, ok := ret.Get(0).(func(uint, flow.Identifier) []messages.BroadcastDKGMessage); ok {
		r0 = rf(fromIndex, referenceBlock)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]messages.BroadcastDKGMessage)
		}
	}

	if rf, ok := ret.Get(1).(func(uint, flow.Identifier) error); ok {
		r1 = rf(fromIndex, referenceBlock)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// SubmitResult provides a mock function with given fields: _a0, _a1
func (_m *DKGContractClient) SubmitResult(_a0 crypto.PublicKey, _a1 []crypto.PublicKey) error {
	ret := _m.Called(_a0, _a1)

	var r0 error
	if rf, ok := ret.Get(0).(func(crypto.PublicKey, []crypto.PublicKey) error); ok {
		r0 = rf(_a0, _a1)
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

type mockConstructorTestingTNewDKGContractClient interface {
	mock.TestingT
	Cleanup(func())
}

// NewDKGContractClient creates a new instance of DKGContractClient. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewDKGContractClient(t mockConstructorTestingTNewDKGContractClient) *DKGContractClient {
	mock := &DKGContractClient{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
