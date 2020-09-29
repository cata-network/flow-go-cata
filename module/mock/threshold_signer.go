// Code generated by mockery v1.0.0. DO NOT EDIT.

package mock

import (
	crypto "github.com/onflow/flow-go/crypto"
	mock "github.com/stretchr/testify/mock"
)

// ThresholdSigner is an autogenerated mock type for the ThresholdSigner type
type ThresholdSigner struct {
	mock.Mock
}

// Combine provides a mock function with given fields: size, shares, indices
func (_m *ThresholdSigner) Combine(size uint, shares []crypto.Signature, indices []uint) (crypto.Signature, error) {
	ret := _m.Called(size, shares, indices)

	var r0 crypto.Signature
	if rf, ok := ret.Get(0).(func(uint, []crypto.Signature, []uint) crypto.Signature); ok {
		r0 = rf(size, shares, indices)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(crypto.Signature)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(uint, []crypto.Signature, []uint) error); ok {
		r1 = rf(size, shares, indices)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Sign provides a mock function with given fields: msg
func (_m *ThresholdSigner) Sign(msg []byte) (crypto.Signature, error) {
	ret := _m.Called(msg)

	var r0 crypto.Signature
	if rf, ok := ret.Get(0).(func([]byte) crypto.Signature); ok {
		r0 = rf(msg)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(crypto.Signature)
		}
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]byte) error); ok {
		r1 = rf(msg)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// Verify provides a mock function with given fields: msg, sig, key
func (_m *ThresholdSigner) Verify(msg []byte, sig crypto.Signature, key crypto.PublicKey) (bool, error) {
	ret := _m.Called(msg, sig, key)

	var r0 bool
	if rf, ok := ret.Get(0).(func([]byte, crypto.Signature, crypto.PublicKey) bool); ok {
		r0 = rf(msg, sig, key)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]byte, crypto.Signature, crypto.PublicKey) error); ok {
		r1 = rf(msg, sig, key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// VerifyThreshold provides a mock function with given fields: msg, sig, key
func (_m *ThresholdSigner) VerifyThreshold(msg []byte, sig crypto.Signature, key crypto.PublicKey) (bool, error) {
	ret := _m.Called(msg, sig, key)

	var r0 bool
	if rf, ok := ret.Get(0).(func([]byte, crypto.Signature, crypto.PublicKey) bool); ok {
		r0 = rf(msg, sig, key)
	} else {
		r0 = ret.Get(0).(bool)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func([]byte, crypto.Signature, crypto.PublicKey) error); ok {
		r1 = rf(msg, sig, key)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}
