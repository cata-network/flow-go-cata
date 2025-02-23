package fvm_test

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/onflow/cadence"
	jsoncdc "github.com/onflow/cadence/encoding/json"
	"github.com/onflow/cadence/runtime/format"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/engine/execution/state/delta"
	"github.com/onflow/flow-go/engine/execution/testutil"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/state"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/utils/unittest"
)

type errorOnAddressSnapshotWrapper struct {
	view  state.View
	owner flow.Address
}

func (s errorOnAddressSnapshotWrapper) Get(
	id flow.RegisterID,
) (
	flow.RegisterValue,
	error,
) {
	// return error if id.Owner is the same as the owner of the wrapper
	if id.Owner == string(s.owner.Bytes()) {
		return nil, fmt.Errorf("error getting register %s", id)
	}
	// fetch from underlying view if set
	if s.view != nil {
		return s.view.Get(id)
	}
	return nil, nil
}

func createAccount(
	t *testing.T,
	vm fvm.VM,
	chain flow.Chain,
	ctx fvm.Context,
	view state.View,
) flow.Address {
	ctx = fvm.NewContextFromParent(
		ctx,
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	)

	txBody := flow.NewTransactionBody().
		SetScript([]byte(createAccountTransaction)).
		AddAuthorizer(chain.ServiceAddress())

	tx := fvm.Transaction(txBody, 0)

	err := vm.Run(ctx, tx, view)
	require.NoError(t, err)
	require.NoError(t, tx.Err)

	accountCreatedEvents := filterAccountCreatedEvents(tx.Events)

	require.Len(t, accountCreatedEvents, 1)

	data, err := jsoncdc.Decode(nil, accountCreatedEvents[0].Payload)
	require.NoError(t, err)
	address := flow.ConvertAddress(
		data.(cadence.Event).Fields[0].(cadence.Address))

	return address
}

type accountKeyAPIVersion string

const (
	accountKeyAPIVersionV1 accountKeyAPIVersion = "V1"
	accountKeyAPIVersionV2 accountKeyAPIVersion = "V2"
)

func addAccountKey(
	t *testing.T,
	vm fvm.VM,
	ctx fvm.Context,
	view state.View,
	address flow.Address,
	apiVersion accountKeyAPIVersion,
) flow.AccountPublicKey {

	privateKey, err := unittest.AccountKeyDefaultFixture()
	require.NoError(t, err)

	publicKeyA, cadencePublicKey := newAccountKey(t, privateKey, apiVersion)

	var addAccountKeyTx accountKeyAPIVersion
	if apiVersion == accountKeyAPIVersionV1 {
		addAccountKeyTx = addAccountKeyTransaction
	} else {
		addAccountKeyTx = addAccountKeyTransactionV2
	}

	txBody := flow.NewTransactionBody().
		SetScript([]byte(addAccountKeyTx)).
		AddArgument(cadencePublicKey).
		AddAuthorizer(address)

	tx := fvm.Transaction(txBody, 0)

	err = vm.Run(ctx, tx, view)
	require.NoError(t, err)
	require.NoError(t, tx.Err)

	return publicKeyA
}

func addAccountCreator(
	t *testing.T,
	vm fvm.VM,
	chain flow.Chain,
	ctx fvm.Context,
	view state.View,
	account flow.Address,
) {
	script := []byte(
		fmt.Sprintf(addAccountCreatorTransactionTemplate,
			chain.ServiceAddress().String(),
			account.String(),
		),
	)

	txBody := flow.NewTransactionBody().
		SetScript(script).
		AddAuthorizer(chain.ServiceAddress())

	tx := fvm.Transaction(txBody, 0)

	err := vm.Run(ctx, tx, view)
	require.NoError(t, err)
	require.NoError(t, tx.Err)
}

func removeAccountCreator(
	t *testing.T,
	vm fvm.VM,
	chain flow.Chain,
	ctx fvm.Context,
	view state.View,
	account flow.Address,
) {
	script := []byte(
		fmt.Sprintf(
			removeAccountCreatorTransactionTemplate,
			chain.ServiceAddress(),
			account.String(),
		),
	)

	txBody := flow.NewTransactionBody().
		SetScript(script).
		AddAuthorizer(chain.ServiceAddress())

	tx := fvm.Transaction(txBody, 0)

	err := vm.Run(ctx, tx, view)
	require.NoError(t, err)
	require.NoError(t, tx.Err)
}

const createAccountTransaction = `
transaction {
  prepare(signer: AuthAccount) {
    let account = AuthAccount(payer: signer)
  }
}
`

const createMultipleAccountsTransaction = `
transaction {
  prepare(signer: AuthAccount) {
    let accountA = AuthAccount(payer: signer)
    let accountB = AuthAccount(payer: signer)
    let accountC = AuthAccount(payer: signer)
  }
}
`

const addAccountKeyTransaction = `
transaction(key: [UInt8]) {
  prepare(signer: AuthAccount) {
    signer.addPublicKey(key)
  }
}
`
const addAccountKeyTransactionV2 = `
transaction(key: [UInt8]) {
  prepare(signer: AuthAccount) {
    let publicKey = PublicKey(
	  publicKey: key,
	  signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
	)
    signer.keys.add(
      publicKey: publicKey,
      hashAlgorithm: HashAlgorithm.SHA3_256,
      weight: 1000.0
    )
  }
}
`

const addMultipleAccountKeysTransaction = `
transaction(key1: [UInt8], key2: [UInt8]) {
  prepare(signer: AuthAccount) {
    signer.addPublicKey(key1)
    signer.addPublicKey(key2)
  }
}
`

const addMultipleAccountKeysTransactionV2 = `
transaction(key1: [UInt8], key2: [UInt8]) {
  prepare(signer: AuthAccount) {
    for key in [key1, key2] {
      let publicKey = PublicKey(
	    publicKey: key,
	    signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
	  )
      signer.keys.add(
        publicKey: publicKey,
        hashAlgorithm: HashAlgorithm.SHA3_256,
        weight: 1000.0
      )
    }
  }
}
`

const removeAccountKeyTransaction = `
transaction(key: Int) {
  prepare(signer: AuthAccount) {
    signer.removePublicKey(key)
  }
}
`

const revokeAccountKeyTransaction = `
transaction(keyIndex: Int) {
  prepare(signer: AuthAccount) {
    signer.keys.revoke(keyIndex: keyIndex)
  }
}
`

const removeMultipleAccountKeysTransaction = `
transaction(key1: Int, key2: Int) {
  prepare(signer: AuthAccount) {
    signer.removePublicKey(key1)
    signer.removePublicKey(key2)
  }
}
`

const revokeMultipleAccountKeysTransaction = `
transaction(keyIndex1: Int, keyIndex2: Int) {
  prepare(signer: AuthAccount) {
    for keyIndex in [keyIndex1, keyIndex2] {
      signer.keys.revoke(keyIndex: keyIndex)
    }
  }
}
`

const removeAccountCreatorTransactionTemplate = `
import FlowServiceAccount from 0x%s
transaction {
	let serviceAccountAdmin: &FlowServiceAccount.Administrator
	prepare(signer: AuthAccount) {
		// Borrow reference to FlowServiceAccount Administrator resource.
		//
		self.serviceAccountAdmin = signer.borrow<&FlowServiceAccount.Administrator>(from: /storage/flowServiceAdmin)
			?? panic("Unable to borrow reference to administrator resource")
	}
	execute {
		// Remove account from account creator allowlist.
		//
		// Will emit AccountCreatorRemoved(accountCreator: accountCreator).
		//
		self.serviceAccountAdmin.removeAccountCreator(0x%s)
	}
}
`

const addAccountCreatorTransactionTemplate = `
import FlowServiceAccount from 0x%s
transaction {
	let serviceAccountAdmin: &FlowServiceAccount.Administrator
	prepare(signer: AuthAccount) {
		// Borrow reference to FlowServiceAccount Administrator resource.
		//
		self.serviceAccountAdmin = signer.borrow<&FlowServiceAccount.Administrator>(from: /storage/flowServiceAdmin)
			?? panic("Unable to borrow reference to administrator resource")
	}
	execute {
		// Add account to account creator allowlist.
		//
		// Will emit AccountCreatorAdded(accountCreator: accountCreator).
		//
		self.serviceAccountAdmin.addAccountCreator(0x%s)
	}
}
`

const getAccountKeyTransaction = `
transaction(keyIndex: Int) {
  prepare(signer: AuthAccount) {
    var key :AccountKey? = signer.keys.get(keyIndex: keyIndex)
    log(key)
  }
}
`

const getMultipleAccountKeysTransaction = `
transaction(keyIndex1: Int, keyIndex2: Int) {
  prepare(signer: AuthAccount) {
    for keyIndex in [keyIndex1, keyIndex2] {
      var key :AccountKey? = signer.keys.get(keyIndex: keyIndex)
      log(key)
    }
  }
}
`

func newAccountKey(
	tb testing.TB,
	privateKey *flow.AccountPrivateKey,
	apiVersion accountKeyAPIVersion,
) (
	publicKey flow.AccountPublicKey,
	encodedCadencePublicKey []byte,
) {
	publicKey = privateKey.PublicKey(fvm.AccountKeyWeightThreshold)

	var publicKeyBytes []byte
	if apiVersion == accountKeyAPIVersionV1 {
		var err error
		publicKeyBytes, err = flow.EncodeRuntimeAccountPublicKey(publicKey)
		require.NoError(tb, err)
	} else {
		publicKeyBytes = publicKey.PublicKey.Encode()
	}

	cadencePublicKey := testutil.BytesToCadenceArray(publicKeyBytes)
	encodedCadencePublicKey, err := jsoncdc.Encode(cadencePublicKey)
	require.NoError(tb, err)

	return publicKey, encodedCadencePublicKey
}

func TestCreateAccount(t *testing.T) {

	options := []fvm.Option{
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	}

	t.Run("Single account",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				payer := createAccount(t, vm, chain, ctx, view)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(createAccountTransaction)).
					AddAuthorizer(payer)

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				assert.NoError(t, tx.Err)

				accountCreatedEvents := filterAccountCreatedEvents(tx.Events)
				require.Len(t, accountCreatedEvents, 1)

				data, err := jsoncdc.Decode(nil, accountCreatedEvents[0].Payload)
				require.NoError(t, err)
				address := flow.ConvertAddress(
					data.(cadence.Event).Fields[0].(cadence.Address))

				account, err := vm.GetAccount(ctx, address, view)
				require.NoError(t, err)
				require.NotNil(t, account)
			}),
	)

	t.Run("Multiple accounts",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				const count = 3

				payer := createAccount(t, vm, chain, ctx, view)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(createMultipleAccountsTransaction)).
					AddAuthorizer(payer)

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				assert.NoError(t, tx.Err)

				accountCreatedEventCount := 0
				for i := 0; i < len(tx.Events); i++ {
					if tx.Events[i].Type != flow.EventAccountCreated {
						continue
					}
					accountCreatedEventCount += 1

					data, err := jsoncdc.Decode(nil, tx.Events[i].Payload)
					require.NoError(t, err)
					address := flow.ConvertAddress(
						data.(cadence.Event).Fields[0].(cadence.Address))

					account, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					require.NotNil(t, account)
				}
				require.Equal(t, count, accountCreatedEventCount)
			}),
	)
}

func TestCreateAccount_WithRestrictedAccountCreation(t *testing.T) {

	options := []fvm.Option{
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	}

	t.Run("Unauthorized account payer",
		newVMTest().
			withContextOptions(options...).
			withBootstrapProcedureOptions(fvm.WithRestrictedAccountCreationEnabled(true)).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				payer := createAccount(t, vm, chain, ctx, view)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(createAccountTransaction)).
					AddAuthorizer(payer)

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				assert.Error(t, tx.Err)
			}),
	)

	t.Run("Authorized account payer",
		newVMTest().withContextOptions(options...).
			withBootstrapProcedureOptions(fvm.WithRestrictedAccountCreationEnabled(true)).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				txBody := flow.NewTransactionBody().
					SetScript([]byte(createAccountTransaction)).
					AddAuthorizer(chain.ServiceAddress())

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				assert.NoError(t, tx.Err)
			}),
	)

	t.Run("Account payer added to allowlist",
		newVMTest().withContextOptions(options...).
			withBootstrapProcedureOptions(fvm.WithRestrictedAccountCreationEnabled(true)).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				payer := createAccount(t, vm, chain, ctx, view)
				addAccountCreator(t, vm, chain, ctx, view, payer)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(createAccountTransaction)).
					SetPayer(payer).
					AddAuthorizer(payer)

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				assert.NoError(t, tx.Err)
			}),
	)

	t.Run("Account payer removed from allowlist",
		newVMTest().withContextOptions(options...).
			withBootstrapProcedureOptions(fvm.WithRestrictedAccountCreationEnabled(true)).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				payer := createAccount(t, vm, chain, ctx, view)
				addAccountCreator(t, vm, chain, ctx, view, payer)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(createAccountTransaction)).
					AddAuthorizer(payer)

				validTx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, validTx, view)
				require.NoError(t, err)

				assert.NoError(t, validTx.Err)

				removeAccountCreator(t, vm, chain, ctx, view, payer)

				invalidTx := fvm.Transaction(txBody, 0)

				err = vm.Run(ctx, invalidTx, view)
				require.NoError(t, err)

				assert.Error(t, invalidTx.Err)
			}),
	)
}

func TestAddAccountKey(t *testing.T) {

	options := []fvm.Option{
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	}

	type addKeyTest struct {
		source     string
		apiVersion accountKeyAPIVersion
	}

	// Add a single key

	singleKeyTests := []addKeyTest{
		{
			source:     addAccountKeyTransaction,
			apiVersion: accountKeyAPIVersionV1,
		},
		{
			source:     addAccountKeyTransactionV2,
			apiVersion: accountKeyAPIVersionV2,
		},
	}

	for _, test := range singleKeyTests {

		t.Run(fmt.Sprintf("Add to empty key list %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Empty(t, before.Keys)

					privateKey, err := unittest.AccountKeyDefaultFixture()
					require.NoError(t, err)

					publicKeyA, cadencePublicKey := newAccountKey(t, privateKey, test.apiVersion)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(cadencePublicKey).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)

					require.Len(t, after.Keys, 1)

					publicKeyB := after.Keys[0]

					assert.Equal(t, publicKeyA.PublicKey, publicKeyB.PublicKey)
					assert.Equal(t, publicKeyA.SignAlgo, publicKeyB.SignAlgo)
					assert.Equal(t, publicKeyA.HashAlgo, publicKeyB.HashAlgo)
					assert.Equal(t, publicKeyA.Weight, publicKeyB.Weight)
				}),
		)

		t.Run(fmt.Sprintf("Add to non-empty key list %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					publicKey1 := addAccountKey(t, vm, ctx, view, address, test.apiVersion)

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, before.Keys, 1)

					privateKey, err := unittest.AccountKeyDefaultFixture()
					require.NoError(t, err)

					publicKey2, publicKey2Arg := newAccountKey(t, privateKey, test.apiVersion)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(publicKey2Arg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)

					expectedKeys := []flow.AccountPublicKey{
						publicKey1,
						publicKey2,
					}

					require.Len(t, after.Keys, len(expectedKeys))

					for i, expectedKey := range expectedKeys {
						actualKey := after.Keys[i]
						assert.Equal(t, i, actualKey.Index)
						assert.Equal(t, expectedKey.PublicKey, actualKey.PublicKey)
						assert.Equal(t, expectedKey.SignAlgo, actualKey.SignAlgo)
						assert.Equal(t, expectedKey.HashAlgo, actualKey.HashAlgo)
						assert.Equal(t, expectedKey.Weight, actualKey.Weight)
					}
				}),
		)

		t.Run(fmt.Sprintf("Invalid key %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					invalidPublicKey := testutil.BytesToCadenceArray([]byte{1, 2, 3})
					invalidPublicKeyArg, err := jsoncdc.Encode(invalidPublicKey)
					require.NoError(t, err)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(invalidPublicKeyArg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.Error(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)

					assert.Empty(t, after.Keys)
				}),
		)
	}

	// Add multiple keys

	multipleKeysTests := []addKeyTest{
		{
			source:     addMultipleAccountKeysTransaction,
			apiVersion: accountKeyAPIVersionV1,
		},
		{
			source:     addMultipleAccountKeysTransactionV2,
			apiVersion: accountKeyAPIVersionV2,
		},
	}

	for _, test := range multipleKeysTests {
		t.Run(fmt.Sprintf("Multiple keys %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Empty(t, before.Keys)

					privateKey1, err := unittest.AccountKeyDefaultFixture()
					require.NoError(t, err)

					privateKey2, err := unittest.AccountKeyDefaultFixture()
					require.NoError(t, err)

					publicKey1, publicKey1Arg := newAccountKey(t, privateKey1, test.apiVersion)
					publicKey2, publicKey2Arg := newAccountKey(t, privateKey2, test.apiVersion)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(publicKey1Arg).
						AddArgument(publicKey2Arg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)

					expectedKeys := []flow.AccountPublicKey{
						publicKey1,
						publicKey2,
					}

					require.Len(t, after.Keys, len(expectedKeys))

					for i, expectedKey := range expectedKeys {
						actualKey := after.Keys[i]
						assert.Equal(t, expectedKey.PublicKey, actualKey.PublicKey)
						assert.Equal(t, expectedKey.SignAlgo, actualKey.SignAlgo)
						assert.Equal(t, expectedKey.HashAlgo, actualKey.HashAlgo)
						assert.Equal(t, expectedKey.Weight, actualKey.Weight)
					}
				}),
		)
	}

	t.Run("Invalid hash algorithms", func(t *testing.T) {

		for _, hashAlgo := range []string{"SHA2_384", "SHA3_384"} {

			t.Run(hashAlgo,
				newVMTest().withContextOptions(options...).
					run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
						address := createAccount(t, vm, chain, ctx, view)

						privateKey, err := unittest.AccountKeyDefaultFixture()
						require.NoError(t, err)

						_, publicKeyArg := newAccountKey(t, privateKey, accountKeyAPIVersionV2)

						txBody := flow.NewTransactionBody().
							SetScript([]byte(fmt.Sprintf(
								`
								transaction(key: [UInt8]) {
								  prepare(signer: AuthAccount) {
								    let publicKey = PublicKey(
									  publicKey: key,
									  signatureAlgorithm: SignatureAlgorithm.ECDSA_P256
									)
								    signer.keys.add(
								      publicKey: publicKey,
								      hashAlgorithm: HashAlgorithm.%s,
								      weight: 1000.0
								    )
								  }
								}
								`,
								hashAlgo,
							))).
							AddArgument(publicKeyArg).
							AddAuthorizer(address)

						tx := fvm.Transaction(txBody, 0)

						err = vm.Run(ctx, tx, view)
						require.NoError(t, err)

						require.Error(t, tx.Err)
						assert.Contains(t, tx.Err.Error(), "hashing algorithm type not supported")

						after, err := vm.GetAccount(ctx, address, view)
						require.NoError(t, err)

						assert.Empty(t, after.Keys)
					}),
			)
		}
	})
}

func TestRemoveAccountKey(t *testing.T) {

	options := []fvm.Option{
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	}

	type removeKeyTest struct {
		source      string
		apiVersion  accountKeyAPIVersion
		expectError bool
	}

	// Remove a single key

	singleKeyTests := []removeKeyTest{
		{
			source:      removeAccountKeyTransaction,
			apiVersion:  accountKeyAPIVersionV1,
			expectError: true,
		},
		{
			source:      revokeAccountKeyTransaction,
			apiVersion:  accountKeyAPIVersionV2,
			expectError: false,
		},
	}

	for _, test := range singleKeyTests {

		t.Run(fmt.Sprintf("Non-existent key %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					const keyCount = 2

					for i := 0; i < keyCount; i++ {
						_ = addAccountKey(t, vm, ctx, view, address, test.apiVersion)
					}

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, before.Keys, keyCount)

					for _, keyIndex := range []int{-1, keyCount, keyCount + 1} {
						keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
						require.NoError(t, err)

						txBody := flow.NewTransactionBody().
							SetScript([]byte(test.source)).
							AddArgument(keyIndexArg).
							AddAuthorizer(address)

						tx := fvm.Transaction(txBody, 0)

						err = vm.Run(ctx, tx, view)
						require.NoError(t, err)

						if test.expectError {
							assert.Error(t, tx.Err)
						} else {
							assert.NoError(t, tx.Err)
						}
					}

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, after.Keys, keyCount)

					for _, publicKey := range after.Keys {
						assert.False(t, publicKey.Revoked)
					}
				}),
		)

		t.Run(fmt.Sprintf("Existing key %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					const keyCount = 2
					const keyIndex = keyCount - 1

					for i := 0; i < keyCount; i++ {
						_ = addAccountKey(t, vm, ctx, view, address, test.apiVersion)
					}

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, before.Keys, keyCount)

					keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
					require.NoError(t, err)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(keyIndexArg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, after.Keys, keyCount)

					for _, publicKey := range after.Keys[:len(after.Keys)-1] {
						assert.False(t, publicKey.Revoked)
					}

					assert.True(t, after.Keys[keyIndex].Revoked)
				}),
		)

		t.Run(fmt.Sprintf("Key added by a different api version %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					const keyCount = 2
					const keyIndex = keyCount - 1

					// Use one version of API to add the keys, and a different version of the API to revoke the keys.
					var apiVersionForAdding accountKeyAPIVersion
					if test.apiVersion == accountKeyAPIVersionV1 {
						apiVersionForAdding = accountKeyAPIVersionV2
					} else {
						apiVersionForAdding = accountKeyAPIVersionV1
					}

					for i := 0; i < keyCount; i++ {
						_ = addAccountKey(t, vm, ctx, view, address, apiVersionForAdding)
					}

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, before.Keys, keyCount)

					keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
					require.NoError(t, err)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddArgument(keyIndexArg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, after.Keys, keyCount)

					for _, publicKey := range after.Keys[:len(after.Keys)-1] {
						assert.False(t, publicKey.Revoked)
					}

					assert.True(t, after.Keys[keyIndex].Revoked)
				}),
		)
	}

	// Remove multiple keys

	multipleKeysTests := []removeKeyTest{
		{
			source:     removeMultipleAccountKeysTransaction,
			apiVersion: accountKeyAPIVersionV1,
		},
		{
			source:     revokeMultipleAccountKeysTransaction,
			apiVersion: accountKeyAPIVersionV2,
		},
	}

	for _, test := range multipleKeysTests {
		t.Run(fmt.Sprintf("Multiple keys %s", test.apiVersion),
			newVMTest().withContextOptions(options...).
				run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
					address := createAccount(t, vm, chain, ctx, view)

					const keyCount = 2

					for i := 0; i < keyCount; i++ {
						_ = addAccountKey(t, vm, ctx, view, address, test.apiVersion)
					}

					before, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, before.Keys, keyCount)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(test.source)).
						AddAuthorizer(address)

					for i := 0; i < keyCount; i++ {
						keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(i))
						require.NoError(t, err)

						txBody.AddArgument(keyIndexArg)
					}

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)

					assert.NoError(t, tx.Err)

					after, err := vm.GetAccount(ctx, address, view)
					require.NoError(t, err)
					assert.Len(t, after.Keys, keyCount)

					for _, publicKey := range after.Keys {
						assert.True(t, publicKey.Revoked)
					}
				}),
		)
	}
}

func TestGetAccountKey(t *testing.T) {

	options := []fvm.Option{
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
		fvm.WithCadenceLogging(true),
	}

	t.Run("Non-existent key",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := createAccount(t, vm, chain, ctx, view)

				const keyCount = 2

				for i := 0; i < keyCount; i++ {
					_ = addAccountKey(t, vm, ctx, view, address, accountKeyAPIVersionV2)
				}

				before, err := vm.GetAccount(ctx, address, view)
				require.NoError(t, err)
				assert.Len(t, before.Keys, keyCount)

				for _, keyIndex := range []int{-1, keyCount, keyCount + 1} {
					keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
					require.NoError(t, err)

					txBody := flow.NewTransactionBody().
						SetScript([]byte(getAccountKeyTransaction)).
						AddArgument(keyIndexArg).
						AddAuthorizer(address)

					tx := fvm.Transaction(txBody, 0)

					err = vm.Run(ctx, tx, view)
					require.NoError(t, err)
					require.NoError(t, tx.Err)

					require.Len(t, tx.Logs, 1)
					assert.Equal(t, "nil", tx.Logs[0])
				}
			}),
	)

	t.Run("Existing key",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := createAccount(t, vm, chain, ctx, view)

				const keyCount = 2
				const keyIndex = keyCount - 1

				keys := make([]flow.AccountPublicKey, keyCount)
				for i := 0; i < keyCount; i++ {
					keys[i] = addAccountKey(t, vm, ctx, view, address, accountKeyAPIVersionV2)
				}

				before, err := vm.GetAccount(ctx, address, view)
				require.NoError(t, err)
				assert.Len(t, before.Keys, keyCount)

				keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
				require.NoError(t, err)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(getAccountKeyTransaction)).
					AddArgument(keyIndexArg).
					AddAuthorizer(address)

				tx := fvm.Transaction(txBody, 0)

				err = vm.Run(ctx, tx, view)
				require.NoError(t, err)
				require.NoError(t, tx.Err)

				require.Len(t, tx.Logs, 1)

				key := keys[keyIndex]

				expected := fmt.Sprintf(
					"AccountKey("+
						"keyIndex: %d, "+
						"publicKey: PublicKey(publicKey: %s, signatureAlgorithm: SignatureAlgorithm(rawValue: 1)), "+
						"hashAlgorithm: HashAlgorithm(rawValue: 3), "+
						"weight: 1000.00000000, "+
						"isRevoked: false)",
					keyIndex,
					byteSliceToCadenceArrayLiteral(key.PublicKey.Encode()),
				)

				assert.Equal(t, expected, tx.Logs[0])
			}),
	)

	t.Run("Key added by a different api version",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := createAccount(t, vm, chain, ctx, view)

				const keyCount = 2
				const keyIndex = keyCount - 1

				keys := make([]flow.AccountPublicKey, keyCount)
				for i := 0; i < keyCount; i++ {

					// Use the old version of API to add the key
					keys[i] = addAccountKey(t, vm, ctx, view, address, accountKeyAPIVersionV1)
				}

				before, err := vm.GetAccount(ctx, address, view)
				require.NoError(t, err)
				assert.Len(t, before.Keys, keyCount)

				keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(keyIndex))
				require.NoError(t, err)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(getAccountKeyTransaction)).
					AddArgument(keyIndexArg).
					AddAuthorizer(address)

				tx := fvm.Transaction(txBody, 0)

				err = vm.Run(ctx, tx, view)
				require.NoError(t, err)
				require.NoError(t, tx.Err)

				require.Len(t, tx.Logs, 1)

				key := keys[keyIndex]

				expected := fmt.Sprintf(
					"AccountKey("+
						"keyIndex: %d, "+
						"publicKey: PublicKey(publicKey: %s, signatureAlgorithm: SignatureAlgorithm(rawValue: 1)), "+
						"hashAlgorithm: HashAlgorithm(rawValue: 3), "+
						"weight: 1000.00000000, "+
						"isRevoked: false)",
					keyIndex,
					byteSliceToCadenceArrayLiteral(key.PublicKey.Encode()),
				)

				assert.Equal(t, expected, tx.Logs[0])
			}),
	)

	t.Run("Multiple keys",
		newVMTest().withContextOptions(options...).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := createAccount(t, vm, chain, ctx, view)

				const keyCount = 2

				keys := make([]flow.AccountPublicKey, keyCount)
				for i := 0; i < keyCount; i++ {

					keys[i] = addAccountKey(t, vm, ctx, view, address, accountKeyAPIVersionV2)
				}

				before, err := vm.GetAccount(ctx, address, view)
				require.NoError(t, err)
				assert.Len(t, before.Keys, keyCount)

				txBody := flow.NewTransactionBody().
					SetScript([]byte(getMultipleAccountKeysTransaction)).
					AddAuthorizer(address)

				for i := 0; i < keyCount; i++ {
					keyIndexArg, err := jsoncdc.Encode(cadence.NewInt(i))
					require.NoError(t, err)

					txBody.AddArgument(keyIndexArg)
				}

				tx := fvm.Transaction(txBody, 0)

				err = vm.Run(ctx, tx, view)
				require.NoError(t, err)
				require.NoError(t, tx.Err)

				assert.Len(t, tx.Logs, 2)

				for i := 0; i < keyCount; i++ {
					expected := fmt.Sprintf(
						"AccountKey("+
							"keyIndex: %d, "+
							"publicKey: PublicKey(publicKey: %s, signatureAlgorithm: SignatureAlgorithm(rawValue: 1)), "+
							"hashAlgorithm: HashAlgorithm(rawValue: 3), "+
							"weight: 1000.00000000, "+
							"isRevoked: false)",
						i,
						byteSliceToCadenceArrayLiteral(keys[i].PublicKey.Encode()),
					)

					assert.Equal(t, expected, tx.Logs[i])
				}
			}),
	)
}

func byteSliceToCadenceArrayLiteral(bytes []byte) string {
	elements := make([]string, 0, len(bytes))

	for _, b := range bytes {
		elements = append(elements, strconv.Itoa(int(b)))
	}

	return format.Array(elements)
}

func TestAccountBalanceFields(t *testing.T) {
	t.Run("Get balance works",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				account := createAccount(t, vm, chain, ctx, view)

				txBody := transferTokensTx(chain).
					AddArgument(jsoncdc.MustEncode(cadence.UFix64(100_000_000))).
					AddArgument(jsoncdc.MustEncode(cadence.Address(account))).
					AddAuthorizer(chain.ServiceAddress())

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.balance
					}
				`, account.Hex())))

				err = vm.Run(ctx, script, view)

				assert.NoError(t, err)

				assert.Equal(t, cadence.UFix64(100_000_000), script.Value)
			}),
	)

	// TODO - this is because get account + borrow returns
	// empty values instead of failing for an account that doesnt exist
	// this behavior needs to addressed on Cadence side
	t.Run("Get balance returns 0 for accounts that don't exist",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				nonExistentAddress, err := chain.AddressAtIndex(100)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.balance
					}
				`, nonExistentAddress)))

				err = vm.Run(ctx, script, view)

				require.NoError(t, err)
				require.NoError(t, script.Err)
				require.Equal(t, cadence.UFix64(0), script.Value)
			}),
	)

	t.Run("Get balance fails if view returns an error",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := createAccount(t, vm, chain, ctx, view)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.balance
					}
				`, address)))

				view = delta.NewDeltaView(
					errorOnAddressSnapshotWrapper{
						view:  view,
						owner: address,
					})

				err := vm.Run(ctx, script, view)
				require.ErrorContains(
					t,
					err,
					fmt.Sprintf(
						"error getting register %s",
						address.Hex()))
			}),
	)

	t.Run("Get available balance works",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1000_000_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				account := createAccount(t, vm, chain, ctx, view)

				txBody := transferTokensTx(chain).
					AddArgument(jsoncdc.MustEncode(cadence.UFix64(100_000_000))).
					AddArgument(jsoncdc.MustEncode(cadence.Address(account))).
					AddAuthorizer(chain.ServiceAddress())

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.availableBalance
					}
				`, account.Hex())))

				err = vm.Run(ctx, script, view)

				assert.NoError(t, err)
				assert.NoError(t, script.Err)
				assert.Equal(t, cadence.UFix64(9999_3120), script.Value)
			}),
	)

	t.Run("Get available balance fails for accounts that don't exist",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1_000_000_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				nonExistentAddress, err := chain.AddressAtIndex(100)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.availableBalance
					}
				`, nonExistentAddress)))

				err = vm.Run(ctx, script, view)

				require.NoError(t, err)
				require.Error(t, script.Err)
			}),
	)

	t.Run("Get available balance works with minimum balance",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1000_000_000),
			fvm.WithAccountCreationFee(100_000),
			fvm.WithMinimumStorageReservation(100_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				account := createAccount(t, vm, chain, ctx, view)

				txBody := transferTokensTx(chain).
					AddArgument(jsoncdc.MustEncode(cadence.UFix64(100_000_000))).
					AddArgument(jsoncdc.MustEncode(cadence.Address(account))).
					AddAuthorizer(chain.ServiceAddress())

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UFix64 {
						let acc = getAccount(0x%s)
						return acc.availableBalance
					}
				`, account.Hex())))

				err = vm.Run(ctx, script, view)

				assert.NoError(t, err)
				assert.NoError(t, script.Err)

				// Should be 100_000_000 because 100_000 was given to it during account creation and is now locked up
				assert.Equal(t, cadence.UFix64(100_000_000), script.Value)
			}),
	)
}

func TestGetStorageCapacity(t *testing.T) {
	t.Run("Get storage capacity",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1_000_000_000),
			fvm.WithAccountCreationFee(100_000),
			fvm.WithMinimumStorageReservation(100_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				account := createAccount(t, vm, chain, ctx, view)

				txBody := transferTokensTx(chain).
					AddArgument(jsoncdc.MustEncode(cadence.UFix64(100_000_000))).
					AddArgument(jsoncdc.MustEncode(cadence.Address(account))).
					AddAuthorizer(chain.ServiceAddress())

				tx := fvm.Transaction(txBody, 0)

				err := vm.Run(ctx, tx, view)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UInt64 {
						let acc = getAccount(0x%s)
						return acc.storageCapacity
					}
				`, account)))

				err = vm.Run(ctx, script, view)

				require.NoError(t, err)
				require.NoError(t, script.Err)

				require.Equal(t, cadence.UInt64(10_010_000), script.Value)
			}),
	)
	t.Run("Get storage capacity returns 0 for accounts that don't exist",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1_000_000_000),
			fvm.WithAccountCreationFee(100_000),
			fvm.WithMinimumStorageReservation(100_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				nonExistentAddress, err := chain.AddressAtIndex(100)
				require.NoError(t, err)

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UInt64 {
						let acc = getAccount(0x%s)
						return acc.storageCapacity
					}
				`, nonExistentAddress)))

				err = vm.Run(ctx, script, view)

				require.NoError(t, err)
				require.NoError(t, script.Err)
				require.Equal(t, cadence.UInt64(0), script.Value)
			}),
	)
	t.Run("Get storage capacity fails if view returns an error",
		newVMTest().withContextOptions(
			fvm.WithAuthorizationChecksEnabled(false),
			fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
			fvm.WithCadenceLogging(true),
			fvm.WithAccountStorageLimit(false),
		).withBootstrapProcedureOptions(
			fvm.WithStorageMBPerFLOW(1_000_000_000),
			fvm.WithAccountCreationFee(100_000),
			fvm.WithMinimumStorageReservation(100_000),
		).
			run(func(t *testing.T, vm fvm.VM, chain flow.Chain, ctx fvm.Context, view state.View) {
				address := chain.ServiceAddress()

				script := fvm.Script([]byte(fmt.Sprintf(`
					pub fun main(): UInt64 {
						let acc = getAccount(0x%s)
						return acc.storageCapacity
					}
				`, address)))

				newview := delta.NewDeltaView(
					errorOnAddressSnapshotWrapper{
						owner: address,
						view:  view,
					})

				err := vm.Run(ctx, script, newview)
				require.ErrorContains(
					t,
					err,
					fmt.Sprintf(
						"error getting register %s",
						address.Hex()))
			}),
	)
}
