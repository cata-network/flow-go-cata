package fvm_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/crypto"
	"github.com/onflow/flow-go/fvm/environment"
	"github.com/onflow/flow-go/fvm/errors"
	"github.com/onflow/flow-go/fvm/storage"
	"github.com/onflow/flow-go/fvm/storage/testutils"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/utils/unittest"
)

func TestTransactionVerification(t *testing.T) {
	txnState := testutils.NewSimpleTransaction(nil)
	accounts := environment.NewAccounts(txnState)

	// create 2 accounts
	address1 := flow.HexToAddress("1234")
	privKey1, err := unittest.AccountKeyDefaultFixture()
	require.NoError(t, err)

	err = accounts.Create([]flow.AccountPublicKey{privKey1.PublicKey(1000)}, address1)
	require.NoError(t, err)

	address2 := flow.HexToAddress("1235")
	privKey2, err := unittest.AccountKeyDefaultFixture()
	require.NoError(t, err)

	err = accounts.Create([]flow.AccountPublicKey{privKey2.PublicKey(1000)}, address2)
	require.NoError(t, err)

	tx := &flow.TransactionBody{}

	run := func(
		body *flow.TransactionBody,
		ctx fvm.Context,
		txn storage.Transaction,
	) error {
		executor := fvm.Transaction(body, 0).NewExecutor(ctx, txn)
		err := fvm.Run(executor)
		require.NoError(t, err)
		return executor.Output().Err
	}

	t.Run("duplicated authorization signatures", func(t *testing.T) {

		sig := flow.TransactionSignature{
			Address:     address1,
			SignerIndex: 0,
			KeyIndex:    0,
		}

		tx.SetProposalKey(address1, 0, 0)
		tx.SetPayer(address1)

		tx.PayloadSignatures = []flow.TransactionSignature{sig, sig}

		ctx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(true),
			fvm.WithAccountKeyWeightThreshold(1000),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithTransactionBodyExecutionEnabled(false))
		err = run(tx, ctx, txnState)
		require.ErrorContains(
			t,
			err,
			"duplicate signatures are provided for the same key")
	})
	t.Run("duplicated authorization and envelope signatures", func(t *testing.T) {

		sig := flow.TransactionSignature{
			Address:     address1,
			SignerIndex: 0,
			KeyIndex:    0,
		}

		tx.SetProposalKey(address1, 0, 0)
		tx.SetPayer(address1)

		tx.PayloadSignatures = []flow.TransactionSignature{sig}
		tx.EnvelopeSignatures = []flow.TransactionSignature{sig}

		ctx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(true),
			fvm.WithAccountKeyWeightThreshold(1000),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithTransactionBodyExecutionEnabled(false))
		err = run(tx, ctx, txnState)
		require.ErrorContains(
			t,
			err,
			"duplicate signatures are provided for the same key")
	})

	t.Run("invalid envelope signature", func(t *testing.T) {
		tx.SetProposalKey(address1, 0, 0)
		tx.SetPayer(address2)

		// assign a valid payload signature
		hasher1, err := crypto.NewPrefixedHashing(privKey1.HashAlgo, flow.TransactionTagString)
		require.NoError(t, err)
		validSig, err := privKey1.PrivateKey.Sign(tx.PayloadMessage(), hasher1) // valid signature
		require.NoError(t, err)

		sig1 := flow.TransactionSignature{
			Address:     address1,
			SignerIndex: 0,
			KeyIndex:    0,
			Signature:   validSig,
		}

		sig2 := flow.TransactionSignature{
			Address:     address2,
			SignerIndex: 0,
			KeyIndex:    0,
			// invalid signature
		}

		tx.PayloadSignatures = []flow.TransactionSignature{sig1}
		tx.EnvelopeSignatures = []flow.TransactionSignature{sig2}

		ctx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(true),
			fvm.WithAccountKeyWeightThreshold(1000),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithTransactionBodyExecutionEnabled(false))
		err = run(tx, ctx, txnState)
		require.Error(t, err)
		require.True(t, errors.IsInvalidEnvelopeSignatureError(err))
	})

	t.Run("invalid payload signature", func(t *testing.T) {
		tx.SetProposalKey(address1, 0, 0)
		tx.SetPayer(address2)

		sig1 := flow.TransactionSignature{
			Address:     address1,
			SignerIndex: 0,
			KeyIndex:    0,
			// invalid signature
		}

		// assign a valid envelope signature
		hasher2, err := crypto.NewPrefixedHashing(privKey2.HashAlgo, flow.TransactionTagString)
		require.NoError(t, err)
		validSig, err := privKey2.PrivateKey.Sign(tx.EnvelopeMessage(), hasher2) // valid signature
		require.NoError(t, err)

		sig2 := flow.TransactionSignature{
			Address:     address2,
			SignerIndex: 0,
			KeyIndex:    0,
			Signature:   validSig,
		}

		tx.PayloadSignatures = []flow.TransactionSignature{sig1}
		tx.EnvelopeSignatures = []flow.TransactionSignature{sig2}

		ctx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(true),
			fvm.WithAccountKeyWeightThreshold(1000),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithTransactionBodyExecutionEnabled(false))
		err = run(tx, ctx, txnState)
		require.Error(t, err)
		require.True(t, errors.IsInvalidPayloadSignatureError(err))
	})

	t.Run("invalid payload and envelope signatures", func(t *testing.T) {
		// TODO: this test expects a Payload error but should be updated to expect en Envelope error.
		// The test should be updated once the FVM updates the order of validating signatures:
		// envelope needs to be checked first and payload later.
		tx.SetProposalKey(address1, 0, 0)
		tx.SetPayer(address2)

		sig1 := flow.TransactionSignature{
			Address:     address1,
			SignerIndex: 0,
			KeyIndex:    0,
			// invalid signature
		}

		sig2 := flow.TransactionSignature{
			Address:     address2,
			SignerIndex: 0,
			KeyIndex:    0,
			// invalid signature
		}

		tx.PayloadSignatures = []flow.TransactionSignature{sig1}
		tx.EnvelopeSignatures = []flow.TransactionSignature{sig2}

		ctx := fvm.NewContext(
			fvm.WithAuthorizationChecksEnabled(true),
			fvm.WithAccountKeyWeightThreshold(1000),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithTransactionBodyExecutionEnabled(false))
		err = run(tx, ctx, txnState)
		require.Error(t, err)

		// TODO: update to InvalidEnvelopeSignatureError once FVM verifier is updated.
		require.True(t, errors.IsInvalidPayloadSignatureError(err))
	})
}
