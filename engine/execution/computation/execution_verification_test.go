package computation

import (
	"context"
	"crypto/rand"
	"fmt"
	"testing"

	"github.com/ipfs/go-datastore"
	dssync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/onflow/cadence"
	jsoncdc "github.com/onflow/cadence/encoding/json"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/onflow/flow-go/crypto"
	"github.com/onflow/flow-go/engine/execution"
	"github.com/onflow/flow-go/engine/execution/computation/committer"
	"github.com/onflow/flow-go/engine/execution/computation/computer"
	"github.com/onflow/flow-go/engine/execution/state"
	bootstrapexec "github.com/onflow/flow-go/engine/execution/state/bootstrap"
	"github.com/onflow/flow-go/engine/execution/testutil"
	"github.com/onflow/flow-go/engine/execution/utils"
	"github.com/onflow/flow-go/engine/testutil/mocklocal"
	"github.com/onflow/flow-go/engine/verification/fetcher"
	"github.com/onflow/flow-go/fvm"
	"github.com/onflow/flow-go/fvm/blueprints"
	"github.com/onflow/flow-go/fvm/derived"
	"github.com/onflow/flow-go/fvm/environment"
	"github.com/onflow/flow-go/fvm/errors"
	completeLedger "github.com/onflow/flow-go/ledger/complete"
	"github.com/onflow/flow-go/ledger/complete/wal/fixtures"
	"github.com/onflow/flow-go/model/flow"
	"github.com/onflow/flow-go/model/verification"
	"github.com/onflow/flow-go/module/chunks"
	"github.com/onflow/flow-go/module/executiondatasync/execution_data"
	exedataprovider "github.com/onflow/flow-go/module/executiondatasync/provider"
	mocktracker "github.com/onflow/flow-go/module/executiondatasync/tracker/mock"
	"github.com/onflow/flow-go/module/metrics"
	requesterunit "github.com/onflow/flow-go/module/state_synchronization/requester/unittest"
	"github.com/onflow/flow-go/module/trace"
	"github.com/onflow/flow-go/utils/unittest"
)

var chain = flow.Emulator.Chain()

// In the following tests the system transaction is expected to fail, as the epoch related things are not set up properly.
// This is not relevant to the test, as only the non-system transactions are tested.

func Test_ExecutionMatchesVerification(t *testing.T) {
	t.Run("empty block", func(t *testing.T) {
		executeBlockAndVerify(t,
			[][]*flow.TransactionBody{},
			fvm.DefaultTransactionFees,
			fvm.DefaultMinimumStorageReservation)
	})

	t.Run("single transaction event", func(t *testing.T) {

		deployTx := blueprints.DeployContractTransaction(chain.ServiceAddress(), []byte(""+
			`pub contract Foo {
				pub event FooEvent(x: Int, y: Int)

				pub fun event() { 
					emit FooEvent(x: 2, y: 1)
				}
			}`), "Foo")

		emitTx := &flow.TransactionBody{
			Script: []byte(fmt.Sprintf(`
			import Foo from 0x%s
			transaction {
				prepare() {}
				execute {
					Foo.event()
				}
			}`, chain.ServiceAddress())),
		}

		err := testutil.SignTransactionAsServiceAccount(deployTx, 0, chain)
		require.NoError(t, err)

		err = testutil.SignTransactionAsServiceAccount(emitTx, 1, chain)
		require.NoError(t, err)

		cr := executeBlockAndVerify(t, [][]*flow.TransactionBody{
			{
				deployTx, emitTx,
			},
		}, fvm.BootstrapProcedureFeeParameters{}, fvm.DefaultMinimumStorageReservation)

		// ensure event is emitted
		require.Empty(t, cr.TransactionResults[0].ErrorMessage)
		require.Empty(t, cr.TransactionResults[1].ErrorMessage)
		require.Len(t, cr.Events[0], 2)
		require.Equal(t, flow.EventType(fmt.Sprintf("A.%s.Foo.FooEvent", chain.ServiceAddress())), cr.Events[0][1].Type)
	})

	t.Run("multiple collections events", func(t *testing.T) {

		deployTx := blueprints.DeployContractTransaction(chain.ServiceAddress(), []byte(""+
			`pub contract Foo {
				pub event FooEvent(x: Int, y: Int)

				pub fun event() { 
					emit FooEvent(x: 2, y: 1)
				}
			}`), "Foo")

		emitTx1 := flow.TransactionBody{
			Script: []byte(fmt.Sprintf(`
			import Foo from 0x%s
			transaction {
				prepare() {}
				execute {
					Foo.event()
				}
			}`, chain.ServiceAddress())),
		}

		// copy txs
		emitTx2 := emitTx1
		emitTx3 := emitTx1

		err := testutil.SignTransactionAsServiceAccount(deployTx, 0, chain)
		require.NoError(t, err)

		err = testutil.SignTransactionAsServiceAccount(&emitTx1, 1, chain)
		require.NoError(t, err)
		err = testutil.SignTransactionAsServiceAccount(&emitTx2, 2, chain)
		require.NoError(t, err)
		err = testutil.SignTransactionAsServiceAccount(&emitTx3, 3, chain)
		require.NoError(t, err)

		cr := executeBlockAndVerify(t, [][]*flow.TransactionBody{
			{
				deployTx, &emitTx1,
			},
			{
				&emitTx2,
			},
			{
				&emitTx3,
			},
		}, fvm.BootstrapProcedureFeeParameters{}, fvm.DefaultMinimumStorageReservation)

		// ensure event is emitted
		require.Empty(t, cr.TransactionResults[0].ErrorMessage)
		require.Empty(t, cr.TransactionResults[1].ErrorMessage)
		require.Empty(t, cr.TransactionResults[2].ErrorMessage)
		require.Empty(t, cr.TransactionResults[3].ErrorMessage)
		require.Len(t, cr.Events[0], 2)
		require.Equal(t, flow.EventType(fmt.Sprintf("A.%s.Foo.FooEvent", chain.ServiceAddress())), cr.Events[0][1].Type)
	})

	t.Run("with failed storage limit", func(t *testing.T) {

		accountPrivKey, createAccountTx := testutil.CreateAccountCreationTransaction(t, chain)

		// this should return the address of newly created account
		accountAddress, err := chain.AddressAtIndex(5)
		require.NoError(t, err)

		err = testutil.SignTransactionAsServiceAccount(createAccountTx, 0, chain)
		require.NoError(t, err)

		addKeyTx := testutil.CreateAddAnAccountKeyMultipleTimesTransaction(t, &accountPrivKey, 100).AddAuthorizer(accountAddress)
		err = testutil.SignTransaction(addKeyTx, accountAddress, accountPrivKey, 0)
		require.NoError(t, err)

		minimumStorage, err := cadence.NewUFix64("0.00010807")
		require.NoError(t, err)

		cr := executeBlockAndVerify(t, [][]*flow.TransactionBody{
			{
				createAccountTx,
			},
			{
				addKeyTx,
			},
		}, fvm.DefaultTransactionFees, minimumStorage)

		// storage limit error
		assert.Equal(t, cr.TransactionResults[0].ErrorMessage, "")
		// ensure events from the first transaction is emitted
		require.Len(t, cr.Events[0], 10)
		// ensure fee deduction events are emitted even though tx fails
		require.Len(t, cr.Events[1], 3)
		// storage limit error
		assert.Contains(t, cr.TransactionResults[1].ErrorMessage, errors.ErrCodeStorageCapacityExceeded.String())
	})

	t.Run("with failed transaction fee deduction", func(t *testing.T) {
		accountPrivKey, createAccountTx := testutil.CreateAccountCreationTransaction(t, chain)
		// this should return the address of newly created account
		accountAddress, err := chain.AddressAtIndex(5)
		require.NoError(t, err)

		err = testutil.SignTransactionAsServiceAccount(createAccountTx, 0, chain)
		require.NoError(t, err)

		spamTx := &flow.TransactionBody{
			Script: []byte(`
			transaction {
				prepare() {}
				execute {
					var s: Int256 = 1024102410241024
					var i = 0
					var a = Int256(7)
					var b = Int256(5)
					var c = Int256(2)
					while i < 150000 {
						s = s * a
						s = s / b
						s = s / c
						i = i + 1
					}
					log(i)
				}
			}`),
		}

		spamTx.SetGasLimit(800000)
		err = testutil.SignTransaction(spamTx, accountAddress, accountPrivKey, 0)
		require.NoError(t, err)

		require.NoError(t, err)

		cr := executeBlockAndVerifyWithParameters(t, [][]*flow.TransactionBody{
			{
				createAccountTx,
				spamTx,
			},
		},
			[]fvm.Option{
				fvm.WithTransactionFeesEnabled(true),
				fvm.WithAccountStorageLimit(true),
				// make sure we don't run out of memory first.
				fvm.WithMemoryLimit(20_000_000_000),
			}, []fvm.BootstrapProcedureOption{
				fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
				fvm.WithAccountCreationFee(fvm.DefaultAccountCreationFee),
				fvm.WithMinimumStorageReservation(fvm.DefaultMinimumStorageReservation),
				fvm.WithTransactionFee(fvm.DefaultTransactionFees),
				fvm.WithStorageMBPerFLOW(fvm.DefaultStorageMBPerFLOW),
			})

		// no error
		assert.Equal(t, cr.TransactionResults[0].ErrorMessage, "")

		// ensure events from the first transaction is emitted. Since transactions are in the same block, get all events from Events[0]
		transactionEvents := 0
		for _, event := range cr.Events[0] {
			if event.TransactionID == cr.TransactionResults[0].TransactionID {
				transactionEvents += 1
			}
		}
		require.Equal(t, 10, transactionEvents)

		assert.Contains(t, cr.TransactionResults[1].ErrorMessage, errors.ErrCodeStorageCapacityExceeded.String())

		// ensure tx fee deduction events are emitted even though tx failed
		transactionEvents = 0
		for _, event := range cr.Events[0] {
			if event.TransactionID == cr.TransactionResults[1].TransactionID {
				transactionEvents += 1
			}
		}
		require.Equal(t, 3, transactionEvents)
	})

}

func TestTransactionFeeDeduction(t *testing.T) {

	type testCase struct {
		name          string
		fundWith      uint64
		tryToTransfer uint64
		checkResult   func(t *testing.T, cr *execution.ComputationResult)
	}

	txFees := uint64(1_000)
	fundingAmount := uint64(100_000_000)
	transferAmount := uint64(123_456)

	testCases := []testCase{
		{
			name:          "Transaction fee deduction emits events",
			fundWith:      fundingAmount,
			tryToTransfer: 0,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Empty(t, cr.TransactionResults[2].ErrorMessage)

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 2)
				require.Len(t, withdraws, 2)
			},
		},
		{
			name:          "If just enough balance, fees are still deducted",
			fundWith:      txFees + transferAmount,
			tryToTransfer: transferAmount,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Empty(t, cr.TransactionResults[2].ErrorMessage)

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 2)
				require.Len(t, withdraws, 2)
			},
		},
		{
			// this is an edge case that is not applicable to any network.
			// If storage limits were on this would fail due to storage limits
			name:          "If not enough balance, transaction succeeds and fees are deducted to 0",
			fundWith:      txFees,
			tryToTransfer: 1,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Empty(t, cr.TransactionResults[2].ErrorMessage)

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 2)
				require.Len(t, withdraws, 2)
			},
		},
		{
			name:          "If tx fails, fees are deducted",
			fundWith:      fundingAmount,
			tryToTransfer: 2 * fundingAmount,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Contains(t, cr.TransactionResults[2].ErrorMessage, "Error Code: 1101")

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 1)
				require.Len(t, withdraws, 1)
			},
		},
	}

	testCasesWithStorageEnabled := []testCase{
		{
			name:          "Transaction fee deduction emits events",
			fundWith:      fundingAmount,
			tryToTransfer: 0,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Empty(t, cr.TransactionResults[2].ErrorMessage)

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 2)
				require.Len(t, withdraws, 2)
			},
		},
		{
			name:          "If just enough balance, fees are deducted",
			fundWith:      txFees + transferAmount,
			tryToTransfer: transferAmount,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Empty(t, cr.TransactionResults[2].ErrorMessage)

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 2)
				require.Len(t, withdraws, 2)
			},
		},
		{
			name:          "If tx fails, fees are still deducted and fee deduction events are emitted",
			fundWith:      fundingAmount,
			tryToTransfer: 2 * fundingAmount,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Contains(t, cr.TransactionResults[2].ErrorMessage, "Error Code: 1101")

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 1)
				require.Len(t, withdraws, 1)
			},
		},
		{
			name:          "If balance at minimum, transaction fails, fees are deducted and fee deduction events are emitted",
			fundWith:      0,
			tryToTransfer: 0,
			checkResult: func(t *testing.T, cr *execution.ComputationResult) {
				require.Empty(t, cr.TransactionResults[0].ErrorMessage)
				require.Empty(t, cr.TransactionResults[1].ErrorMessage)
				require.Contains(t, cr.TransactionResults[2].ErrorMessage, errors.ErrCodeStorageCapacityExceeded.String())

				var deposits []flow.Event
				var withdraws []flow.Event

				for _, e := range cr.Events[2] {
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensDeposited", fvm.FlowTokenAddress(chain)) {
						deposits = append(deposits, e)
					}
					if string(e.Type) == fmt.Sprintf("A.%s.FlowToken.TokensWithdrawn", fvm.FlowTokenAddress(chain)) {
						withdraws = append(withdraws, e)
					}
				}

				require.Len(t, deposits, 1)
				require.Len(t, withdraws, 1)
			},
		},
	}

	transferTokensTx := func(chain flow.Chain) *flow.TransactionBody {
		return flow.NewTransactionBody().
			SetScript([]byte(fmt.Sprintf(`
							// This transaction is a template for a transaction that
							// could be used by anyone to send tokens to another account
							// that has been set up to receive tokens.
							//
							// The withdraw amount and the account from getAccount
							// would be the parameters to the transaction
							
							import FungibleToken from 0x%s
							import FlowToken from 0x%s
							
							transaction(amount: UFix64, to: Address) {
							
								// The Vault resource that holds the tokens that are being transferred
								let sentVault: @FungibleToken.Vault
							
								prepare(signer: AuthAccount) {
							
									// Get a reference to the signer's stored vault
									let vaultRef = signer.borrow<&FlowToken.Vault>(from: /storage/flowTokenVault)
										?? panic("Could not borrow reference to the owner's Vault!")
							
									// Withdraw tokens from the signer's stored vault
									self.sentVault <- vaultRef.withdraw(amount: amount)
								}
							
								execute {
							
									// Get the recipient's public account object
									let recipient = getAccount(to)
							
									// Get a reference to the recipient's Receiver
									let receiverRef = recipient.getCapability(/public/flowTokenReceiver)
										.borrow<&{FungibleToken.Receiver}>()
										?? panic("Could not borrow receiver reference to the recipient's Vault")
							
									// Deposit the withdrawn tokens in the recipient's receiver
									receiverRef.deposit(from: <-self.sentVault)
								}
							}`, fvm.FungibleTokenAddress(chain), fvm.FlowTokenAddress(chain))),
			)
	}

	runTx := func(tc testCase,
		opts []fvm.Option,
		bootstrapOpts []fvm.BootstrapProcedureOption) func(t *testing.T) {
		return func(t *testing.T) {
			// ==== Create an account ====
			privateKey, createAccountTx := testutil.CreateAccountCreationTransaction(t, chain)

			// this should return the address of newly created account
			address, err := chain.AddressAtIndex(5)
			require.NoError(t, err)

			err = testutil.SignTransactionAsServiceAccount(createAccountTx, 0, chain)
			require.NoError(t, err)

			// ==== Transfer tokens to new account ====
			transferTx := transferTokensTx(chain).
				AddAuthorizer(chain.ServiceAddress()).
				AddArgument(jsoncdc.MustEncode(cadence.UFix64(tc.fundWith))).
				AddArgument(jsoncdc.MustEncode(cadence.NewAddress(address)))

			transferTx.SetProposalKey(chain.ServiceAddress(), 0, 1)
			transferTx.SetPayer(chain.ServiceAddress())

			err = testutil.SignEnvelope(
				transferTx,
				chain.ServiceAddress(),
				unittest.ServiceAccountPrivateKey,
			)
			require.NoError(t, err)

			// ==== Transfer tokens from new account ====

			transferTx2 := transferTokensTx(chain).
				AddAuthorizer(address).
				AddArgument(jsoncdc.MustEncode(cadence.UFix64(tc.tryToTransfer))).
				AddArgument(jsoncdc.MustEncode(cadence.NewAddress(chain.ServiceAddress())))

			transferTx2.SetProposalKey(address, 0, 0)
			transferTx2.SetPayer(address)

			err = testutil.SignEnvelope(
				transferTx2,
				address,
				privateKey,
			)
			require.NoError(t, err)

			cr := executeBlockAndVerifyWithParameters(t, [][]*flow.TransactionBody{
				{
					createAccountTx,
				},
				{
					transferTx,
				},
				{
					transferTx2,
				},
			}, opts, bootstrapOpts)

			tc.checkResult(t, cr)
		}
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("Transaction Fees without storage %d: %s", i, tc.name), runTx(tc, []fvm.Option{
			fvm.WithTransactionFeesEnabled(true),
			fvm.WithAccountStorageLimit(false),
		}, []fvm.BootstrapProcedureOption{
			fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
			fvm.WithTransactionFee(fvm.DefaultTransactionFees),
		}))
	}

	for i, tc := range testCasesWithStorageEnabled {
		t.Run(fmt.Sprintf("Transaction Fees with storage %d: %s", i, tc.name), runTx(tc, []fvm.Option{
			fvm.WithTransactionFeesEnabled(true),
			fvm.WithAccountStorageLimit(true),
		}, []fvm.BootstrapProcedureOption{
			fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
			fvm.WithAccountCreationFee(fvm.DefaultAccountCreationFee),
			fvm.WithMinimumStorageReservation(fvm.DefaultMinimumStorageReservation),
			fvm.WithTransactionFee(fvm.DefaultTransactionFees),
			fvm.WithStorageMBPerFLOW(fvm.DefaultStorageMBPerFLOW),
		}))
	}
}

func executeBlockAndVerifyWithParameters(t *testing.T,
	txs [][]*flow.TransactionBody,
	opts []fvm.Option,
	bootstrapOpts []fvm.BootstrapProcedureOption) *execution.ComputationResult {
	vm := fvm.NewVirtualMachine()

	logger := zerolog.Nop()

	opts = append(opts, fvm.WithChain(chain))
	opts = append(opts, fvm.WithLogger(logger))
	opts = append(opts, fvm.WithBlocks(&environment.NoopBlockFinder{}))

	fvmContext :=
		fvm.NewContext(
			opts...,
		)

	collector := metrics.NewNoopCollector()
	tracer := trace.NewNoopTracer()

	wal := &fixtures.NoopWAL{}

	ledger, err := completeLedger.NewLedger(wal, 100, collector, logger, completeLedger.DefaultPathFinderVersion)
	require.NoError(t, err)

	compactor := fixtures.NewNoopCompactor(ledger)
	<-compactor.Ready()
	defer func() {
		<-ledger.Done()
		<-compactor.Done()
	}()

	bootstrapper := bootstrapexec.NewBootstrapper(logger)

	initialCommit, err := bootstrapper.BootstrapLedger(
		ledger,
		unittest.ServiceAccountPublicKey,
		chain,
		bootstrapOpts...,
	)

	require.NoError(t, err)

	ledgerCommiter := committer.NewLedgerViewCommitter(ledger, tracer)

	bservice := requesterunit.MockBlobService(blockstore.NewBlockstore(dssync.MutexWrap(datastore.NewMapDatastore())))
	trackerStorage := mocktracker.NewMockStorage()

	prov := exedataprovider.NewProvider(
		zerolog.Nop(),
		metrics.NewNoopCollector(),
		execution_data.DefaultSerializer,
		bservice,
		trackerStorage,
	)

	// generates signing identity including staking key for signing
	myIdentity := unittest.IdentityFixture()
	seed := make([]byte, crypto.KeyGenSeedMinLenBLSBLS12381)
	n, err := rand.Read(seed)
	require.Equal(t, n, crypto.KeyGenSeedMinLenBLSBLS12381)
	require.NoError(t, err)
	sk, err := crypto.GeneratePrivateKey(crypto.BLSBLS12381, seed)
	require.NoError(t, err)
	myIdentity.StakingPubKey = sk.PublicKey()
	me := mocklocal.NewMockLocal(sk, myIdentity.ID(), t)

	blockComputer, err := computer.NewBlockComputer(
		vm,
		fvmContext,
		collector,
		tracer,
		logger,
		ledgerCommiter,
		me,
		prov,
		nil)
	require.NoError(t, err)

	executableBlock := unittest.ExecutableBlockFromTransactions(chain.ChainID(), txs)
	executableBlock.StartState = &initialCommit

	prevResultId := unittest.IdentifierFixture()
	computationResult, err := blockComputer.ExecuteBlock(
		context.Background(),
		prevResultId,
		executableBlock,
		state.NewLedgerStorageSnapshot(
			ledger,
			initialCommit),
		derived.NewEmptyDerivedBlockData())
	require.NoError(t, err)

	spockHasher := utils.NewSPOCKHasher()
	for i, snapshot := range computationResult.StateSnapshots {
		valid, err := crypto.SPOCKVerifyAgainstData(
			myIdentity.StakingPubKey,
			computationResult.Spocks[i],
			snapshot.SpockSecret,
			spockHasher)
		require.NoError(t, err)
		require.True(t, valid)
	}

	receipt := computationResult.ExecutionReceipt
	receiptID := receipt.ID()
	valid, err := myIdentity.StakingPubKey.Verify(
		receipt.ExecutorSignature,
		receiptID[:],
		utils.NewExecutionReceiptHasher())

	require.NoError(t, err)
	require.True(t, valid)

	require.Equal(t, len(computationResult.ChunkDataPacks), len(receipt.Spocks))

	chdps := computationResult.ChunkDataPacks
	er := &computationResult.ExecutionResult

	verifier := chunks.NewChunkVerifier(vm, fvmContext, logger)

	vcds := make([]*verification.VerifiableChunkData, er.Chunks.Len())

	for i, chunk := range er.Chunks {
		isSystemChunk := i == er.Chunks.Len()-1
		offsetForChunk, err := fetcher.TransactionOffsetForChunk(er.Chunks, chunk.Index)
		require.NoError(t, err)

		vcds[i] = &verification.VerifiableChunkData{
			IsSystemChunk:     isSystemChunk,
			Chunk:             chunk,
			Header:            executableBlock.Block.Header,
			Result:            er,
			ChunkDataPack:     chdps[i],
			EndState:          chunk.EndState,
			TransactionOffset: offsetForChunk,
		}
	}

	require.Len(t, vcds, len(txs)+1) // +1 for system chunk

	for _, vcd := range vcds {
		_, fault, err := verifier.Verify(vcd)
		assert.NoError(t, err)
		assert.Nil(t, fault)
	}

	return computationResult
}

func executeBlockAndVerify(t *testing.T,
	txs [][]*flow.TransactionBody,
	txFees fvm.BootstrapProcedureFeeParameters,
	minStorageBalance cadence.UFix64) *execution.ComputationResult {
	return executeBlockAndVerifyWithParameters(t,
		txs,
		[]fvm.Option{
			fvm.WithTransactionFeesEnabled(true),
			fvm.WithAccountStorageLimit(true),
		}, []fvm.BootstrapProcedureOption{
			fvm.WithInitialTokenSupply(unittest.GenesisTokenSupply),
			fvm.WithAccountCreationFee(fvm.DefaultAccountCreationFee),
			fvm.WithMinimumStorageReservation(minStorageBalance),
			fvm.WithTransactionFee(txFees),
			fvm.WithStorageMBPerFLOW(fvm.DefaultStorageMBPerFLOW),
		})
}
