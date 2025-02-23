package testutil

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"

	"github.com/onflow/cadence"
	jsoncdc "github.com/onflow/cadence/encoding/json"
	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/crypto"
	"github.com/onflow/flow-go/crypto/hash"
	"github.com/onflow/flow-go/engine/execution/state/delta"
	"github.com/onflow/flow-go/engine/execution/utils"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/state"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/module/epochs"
	"github.com/onflow/flow-go/utils/unittest"
)

func CreateContractDeploymentTransaction(contractName string, contract string, authorizer flow.Address, chain flow.Chain) *flow.TransactionBody {

	encoded := hex.EncodeToString([]byte(contract))

	script := []byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount, service: AuthAccount) {
                signer.contracts.add(name: "%s", code: "%s".decodeHex())
              }
            }`, contractName, encoded))

	txBody := flow.NewTransactionBody().
		SetScript(script).
		AddAuthorizer(authorizer).
		AddAuthorizer(chain.ServiceAddress())

	return txBody
}

func UpdateContractDeploymentTransaction(contractName string, contract string, authorizer flow.Address, chain flow.Chain) *flow.TransactionBody {
	encoded := hex.EncodeToString([]byte(contract))

	return flow.NewTransactionBody().
		SetScript([]byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount, service: AuthAccount) {
                signer.contracts.update__experimental(name: "%s", code: "%s".decodeHex())
              }
            }`, contractName, encoded)),
		).
		AddAuthorizer(authorizer).
		AddAuthorizer(chain.ServiceAddress())
}

func UpdateContractUnathorizedDeploymentTransaction(contractName string, contract string, authorizer flow.Address) *flow.TransactionBody {
	encoded := hex.EncodeToString([]byte(contract))

	return flow.NewTransactionBody().
		SetScript([]byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount) {
                signer.contracts.update__experimental(name: "%s", code: "%s".decodeHex())
              }
            }`, contractName, encoded)),
		).
		AddAuthorizer(authorizer)
}

func RemoveContractDeploymentTransaction(contractName string, authorizer flow.Address, chain flow.Chain) *flow.TransactionBody {
	return flow.NewTransactionBody().
		SetScript([]byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount, service: AuthAccount) {
                signer.contracts.remove(name: "%s")
              }
            }`, contractName)),
		).
		AddAuthorizer(authorizer).
		AddAuthorizer(chain.ServiceAddress())
}

func RemoveContractUnathorizedDeploymentTransaction(contractName string, authorizer flow.Address) *flow.TransactionBody {
	return flow.NewTransactionBody().
		SetScript([]byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount) {
                signer.contracts.remove(name: "%s")
              }
            }`, contractName)),
		).
		AddAuthorizer(authorizer)
}

func CreateUnauthorizedContractDeploymentTransaction(contractName string, contract string, authorizer flow.Address) *flow.TransactionBody {
	encoded := hex.EncodeToString([]byte(contract))

	return flow.NewTransactionBody().
		SetScript([]byte(fmt.Sprintf(`transaction {
              prepare(signer: AuthAccount) {
                signer.contracts.add(name: "%s", code: "%s".decodeHex())
              }
            }`, contractName, encoded)),
		).
		AddAuthorizer(authorizer)
}

func SignPayload(
	tx *flow.TransactionBody,
	account flow.Address,
	privateKey flow.AccountPrivateKey,
) error {
	hasher, err := utils.NewHasher(privateKey.HashAlgo)
	if err != nil {
		return fmt.Errorf("failed to create hasher: %w", err)
	}

	err = tx.SignPayload(account, 0, privateKey.PrivateKey, hasher)

	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	return nil
}

func SignEnvelope(tx *flow.TransactionBody, account flow.Address, privateKey flow.AccountPrivateKey) error {
	hasher, err := utils.NewHasher(privateKey.HashAlgo)
	if err != nil {
		return fmt.Errorf("failed to create hasher: %w", err)
	}

	err = tx.SignEnvelope(account, 0, privateKey.PrivateKey, hasher)

	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	return nil
}

func SignTransaction(
	tx *flow.TransactionBody,
	address flow.Address,
	privateKey flow.AccountPrivateKey,
	seqNum uint64,
) error {
	tx.SetProposalKey(address, 0, seqNum)
	tx.SetPayer(address)
	return SignEnvelope(tx, address, privateKey)
}

func SignTransactionAsServiceAccount(tx *flow.TransactionBody, seqNum uint64, chain flow.Chain) error {
	return SignTransaction(tx, chain.ServiceAddress(), unittest.ServiceAccountPrivateKey, seqNum)
}

// GenerateAccountPrivateKeys generates a number of private keys.
func GenerateAccountPrivateKeys(numberOfPrivateKeys int) ([]flow.AccountPrivateKey, error) {
	var privateKeys []flow.AccountPrivateKey
	for i := 0; i < numberOfPrivateKeys; i++ {
		pk, err := GenerateAccountPrivateKey()
		if err != nil {
			return nil, err
		}
		privateKeys = append(privateKeys, pk)
	}

	return privateKeys, nil
}

// GenerateAccountPrivateKey generates a private key.
func GenerateAccountPrivateKey() (flow.AccountPrivateKey, error) {
	seed := make([]byte, crypto.KeyGenSeedMinLenECDSAP256)
	_, err := rand.Read(seed)
	if err != nil {
		return flow.AccountPrivateKey{}, err
	}
	privateKey, err := crypto.GeneratePrivateKey(crypto.ECDSAP256, seed)
	if err != nil {
		return flow.AccountPrivateKey{}, err
	}
	pk := flow.AccountPrivateKey{
		PrivateKey: privateKey,
		SignAlgo:   crypto.ECDSAP256,
		HashAlgo:   hash.SHA2_256,
	}
	return pk, nil
}

// CreateAccounts inserts accounts into the ledger using the provided private keys.
func CreateAccounts(
	vm fvm.VM,
	view state.View,
	privateKeys []flow.AccountPrivateKey,
	chain flow.Chain,
) ([]flow.Address, error) {
	return CreateAccountsWithSimpleAddresses(vm, view, privateKeys, chain)
}

func CreateAccountsWithSimpleAddresses(
	vm fvm.VM,
	view state.View,
	privateKeys []flow.AccountPrivateKey,
	chain flow.Chain,
) ([]flow.Address, error) {
	ctx := fvm.NewContext(
		fvm.WithChain(chain),
		fvm.WithAuthorizationChecksEnabled(false),
		fvm.WithSequenceNumberCheckAndIncrementEnabled(false),
	)

	var accounts []flow.Address

	scriptTemplate := `
        transaction(publicKey: [UInt8]) {
            prepare(signer: AuthAccount) {
                let acct = AuthAccount(payer: signer)
                let publicKey2 = PublicKey(
                    publicKey: publicKey,
                    signatureAlgorithm: SignatureAlgorithm.%s
                )
                acct.keys.add(
                    publicKey: publicKey2,
                    hashAlgorithm: HashAlgorithm.%s,
                    weight: %d.0
                )
            }
	    }`

	serviceAddress := chain.ServiceAddress()

	for _, privateKey := range privateKeys {
		accountKey := privateKey.PublicKey(fvm.AccountKeyWeightThreshold)
		encPublicKey := accountKey.PublicKey.Encode()
		cadPublicKey := BytesToCadenceArray(encPublicKey)
		encCadPublicKey, _ := jsoncdc.Encode(cadPublicKey)

		script := []byte(
			fmt.Sprintf(
				scriptTemplate,
				accountKey.SignAlgo.String(),
				accountKey.HashAlgo.String(),
				accountKey.Weight,
			),
		)

		txBody := flow.NewTransactionBody().
			SetScript(script).
			AddArgument(encCadPublicKey).
			AddAuthorizer(serviceAddress)

		tx := fvm.Transaction(txBody, 0)
		err := vm.Run(ctx, tx, view)
		if err != nil {
			return nil, err
		}

		if tx.Err != nil {
			return nil, fmt.Errorf("failed to create account: %w", tx.Err)
		}

		var addr flow.Address

		for _, event := range tx.Events {
			if event.Type == flow.EventAccountCreated {
				data, err := jsoncdc.Decode(nil, event.Payload)
				if err != nil {
					return nil, errors.New("error decoding events")
				}
				addr = flow.ConvertAddress(
					data.(cadence.Event).Fields[0].(cadence.Address))
				break
			}
		}
		if addr == flow.EmptyAddress {
			return nil, errors.New("no account creation event emitted")
		}
		accounts = append(accounts, addr)
	}

	return accounts, nil
}

func RootBootstrappedLedger(vm fvm.VM, ctx fvm.Context, additionalOptions ...fvm.BootstrapProcedureOption) state.View {
	view := delta.NewDeltaView(nil)

	// set 0 clusters to pass n_collectors >= n_clusters check
	epochConfig := epochs.DefaultEpochConfig()
	epochConfig.NumCollectorClusters = 0

	options := []fvm.BootstrapProcedureOption{
		fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
		fvm.WithEpochConfig(epochConfig),
	}

	options = append(options, additionalOptions...)

	bootstrap := fvm.Bootstrap(
		unittest.ServiceAccountPublicKey,
		options...,
	)

	err := vm.Run(ctx, bootstrap, view)
	if err != nil {
		panic(err)
	}
	return view
}

func BytesToCadenceArray(l []byte) cadence.Array {
	values := make([]cadence.Value, len(l))
	for i, b := range l {
		values[i] = cadence.NewUInt8(b)
	}

	return cadence.NewArray(values).WithType(cadence.NewVariableSizedArrayType(cadence.NewUInt8Type()))
}

// CreateAccountCreationTransaction creates a transaction which will create a new account.
//
// This function returns a randomly generated private key and the transaction.
func CreateAccountCreationTransaction(t testing.TB, chain flow.Chain) (flow.AccountPrivateKey, *flow.TransactionBody) {
	accountKey, err := GenerateAccountPrivateKey()
	require.NoError(t, err)
	encPublicKey := accountKey.PublicKey(1000).PublicKey.Encode()
	cadPublicKey := BytesToCadenceArray(encPublicKey)
	encCadPublicKey, err := jsoncdc.Encode(cadPublicKey)
	require.NoError(t, err)

	// define the cadence script
	script := fmt.Sprintf(`
        transaction(publicKey: [UInt8]) {
            prepare(signer: AuthAccount) {
				let acct = AuthAccount(payer: signer)
                let publicKey2 = PublicKey(
                    publicKey: publicKey,
                    signatureAlgorithm: SignatureAlgorithm.%s
                )
                acct.keys.add(
                    publicKey: publicKey2,
                    hashAlgorithm: HashAlgorithm.%s,
                    weight: 1000.0
                )
            }
	    }`,
		accountKey.SignAlgo.String(),
		accountKey.HashAlgo.String(),
	)

	// create the transaction to create the account
	tx := flow.NewTransactionBody().
		SetScript([]byte(script)).
		AddArgument(encCadPublicKey).
		AddAuthorizer(chain.ServiceAddress())

	return accountKey, tx
}

// CreateMultiAccountCreationTransaction creates a transaction which will create many (n) new account.
//
// This function returns a randomly generated private key and the transaction.
func CreateMultiAccountCreationTransaction(t *testing.T, chain flow.Chain, n int) (flow.AccountPrivateKey, *flow.TransactionBody) {
	accountKey, err := GenerateAccountPrivateKey()
	require.NoError(t, err)
	encPublicKey := accountKey.PublicKey(1000).PublicKey.Encode()
	cadPublicKey := BytesToCadenceArray(encPublicKey)
	encCadPublicKey, err := jsoncdc.Encode(cadPublicKey)
	require.NoError(t, err)

	// define the cadence script
	script := fmt.Sprintf(`
        transaction(publicKey: [UInt8]) {
            prepare(signer: AuthAccount) {
                var i = 0
                while i < %d {
                    let account = AuthAccount(payer: signer)
                    let publicKey2 = PublicKey(
                        publicKey: publicKey,
                        signatureAlgorithm: SignatureAlgorithm.%s
                    )
                    account.keys.add(
                        publicKey: publicKey2,
                        hashAlgorithm: HashAlgorithm.%s,
                        weight: 1000.0
                    )
                    i = i + 1
                }
            }
	    }`,
		n,
		accountKey.SignAlgo.String(),
		accountKey.HashAlgo.String(),
	)

	// create the transaction to create the account
	tx := flow.NewTransactionBody().
		SetScript([]byte(script)).
		AddArgument(encCadPublicKey).
		AddAuthorizer(chain.ServiceAddress())

	return accountKey, tx
}

// CreateAddAnAccountKeyMultipleTimesTransaction generates a tx that adds a key several times to an account.
// this can be used to exhaust an account's storage.
func CreateAddAnAccountKeyMultipleTimesTransaction(t *testing.T, accountKey *flow.AccountPrivateKey, counts int) *flow.TransactionBody {
	script := []byte(fmt.Sprintf(`
      transaction(counts: Int, key: [UInt8]) {
        prepare(signer: AuthAccount) {
          var i = 0
          while i < counts {
            i = i + 1
            let publicKey2 = PublicKey(
              publicKey: key,
              signatureAlgorithm: SignatureAlgorithm.%s
            )
            signer.keys.add(
              publicKey: publicKey2,
              hashAlgorithm: HashAlgorithm.%s,
              weight: 1000.0
            )
	      }
        }
      }
   	`, accountKey.SignAlgo.String(), accountKey.HashAlgo.String()))

	arg1, err := jsoncdc.Encode(cadence.NewInt(counts))
	require.NoError(t, err)

	encPublicKey := accountKey.PublicKey(1000).PublicKey.Encode()
	cadPublicKey := BytesToCadenceArray(encPublicKey)
	arg2, err := jsoncdc.Encode(cadPublicKey)
	require.NoError(t, err)

	addKeysTx := &flow.TransactionBody{
		Script: script,
	}
	addKeysTx = addKeysTx.AddArgument(arg1).AddArgument(arg2)
	return addKeysTx
}

// CreateAddAccountKeyTransaction generates a tx that adds a key to an account.
func CreateAddAccountKeyTransaction(t *testing.T, accountKey *flow.AccountPrivateKey) *flow.TransactionBody {
	keyBytes := accountKey.PublicKey(1000).PublicKey.Encode()

	script := []byte(`
        transaction(key: [UInt8]) {
          prepare(signer: AuthAccount) {
            let acct = AuthAccount(payer: signer)
            let publicKey2 = PublicKey(
              publicKey: key,
              signatureAlgorithm: SignatureAlgorithm.%s
            )
            signer.keys.add(
              publicKey: publicKey2,
              hashAlgorithm: HashAlgorithm.%s,
              weight: %d.0
            )
          }
        }
   	`)

	arg, err := jsoncdc.Encode(bytesToCadenceArray(keyBytes))
	require.NoError(t, err)

	addKeysTx := &flow.TransactionBody{
		Script: script,
	}
	addKeysTx = addKeysTx.AddArgument(arg)

	return addKeysTx
}

func bytesToCadenceArray(l []byte) cadence.Array {
	values := make([]cadence.Value, len(l))
	for i, b := range l {
		values[i] = cadence.NewUInt8(b)
	}

	return cadence.NewArray(values)
}
