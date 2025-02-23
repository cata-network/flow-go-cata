// Code generated by mockery v2.21.4. DO NOT EDIT.

package mocks

import (
	hotstuff "github.com/onflow/flow-go/consensus/hotstuff"
	flow "github.com/onflow/flow-go/model/flow"

	mock "github.com/stretchr/testify/mock"
)

// Packer is an autogenerated mock type for the Packer type
type Packer struct {
	mock.Mock
}

// Pack provides a mock function with given fields: view, sig
func (_m *Packer) Pack(view uint64, sig *hotstuff.BlockSignatureData) ([]byte, []byte, error) {
	ret := _m.Called(view, sig)

	var r0 []byte
	var r1 []byte
	var r2 error
	if rf, ok := ret.Get(0).(func(uint64, *hotstuff.BlockSignatureData) ([]byte, []byte, error)); ok {
		return rf(view, sig)
	}
	if rf, ok := ret.Get(0).(func(uint64, *hotstuff.BlockSignatureData) []byte); ok {
		r0 = rf(view, sig)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).([]byte)
		}
	}

	if rf, ok := ret.Get(1).(func(uint64, *hotstuff.BlockSignatureData) []byte); ok {
		r1 = rf(view, sig)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).([]byte)
		}
	}

	if rf, ok := ret.Get(2).(func(uint64, *hotstuff.BlockSignatureData) error); ok {
		r2 = rf(view, sig)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

// Unpack provides a mock function with given fields: signerIdentities, sigData
func (_m *Packer) Unpack(signerIdentities flow.IdentityList, sigData []byte) (*hotstuff.BlockSignatureData, error) {
	ret := _m.Called(signerIdentities, sigData)

	var r0 *hotstuff.BlockSignatureData
	var r1 error
	if rf, ok := ret.Get(0).(func(flow.IdentityList, []byte) (*hotstuff.BlockSignatureData, error)); ok {
		return rf(signerIdentities, sigData)
	}
	if rf, ok := ret.Get(0).(func(flow.IdentityList, []byte) *hotstuff.BlockSignatureData); ok {
		r0 = rf(signerIdentities, sigData)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*hotstuff.BlockSignatureData)
		}
	}

	if rf, ok := ret.Get(1).(func(flow.IdentityList, []byte) error); ok {
		r1 = rf(signerIdentities, sigData)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

type mockConstructorTestingTNewPacker interface {
	mock.TestingT
	Cleanup(func())
}

// NewPacker creates a new instance of Packer. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewPacker(t mockConstructorTestingTNewPacker) *Packer {
	mock := &Packer{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
