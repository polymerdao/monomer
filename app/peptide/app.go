package peptide

import (
	"context"
	"encoding/json"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	abci "github.com/cometbft/cometbft/abci/types"
	tmjson "github.com/cometbft/cometbft/libs/json"
	tmlog "github.com/cometbft/cometbft/libs/log"
	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	tmtypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	teststaking "github.com/cosmos/cosmos-sdk/x/staking/testutil"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/notbdu/gm/app"
	"github.com/notbdu/gm/app/params"
	"github.com/samber/lo"
)

// PeptideApp extends the ABCI-compatible App with additional op-stack L2 chain features
type PeptideApp struct {
	// App is the ABCI-compatible App
	// TODO: IMPORT YOUR ABCI APP HERE
	*app.App

	ValSet               *tmtypes.ValidatorSet
	EncodingConfig       *params.EncodingConfig
	lastHeader           *tmproto.Header
	currentHeader        *tmproto.Header
	ChainId              string
	BondDenom            string
	VotingPowerReduction sdk.Int
}

func setPrefixes(accountAddressPrefix string) {
	// Set prefixes
	accountPubKeyPrefix := accountAddressPrefix + "pub"
	validatorAddressPrefix := accountAddressPrefix + "valoper"
	validatorPubKeyPrefix := accountAddressPrefix + "valoperpub"
	consNodeAddressPrefix := accountAddressPrefix + "valcons"
	consNodePubKeyPrefix := accountAddressPrefix + "valconspub"

	// Set and seal config
	config := sdk.GetConfig()
	config.SetBech32PrefixForAccount(accountAddressPrefix, accountPubKeyPrefix)
	config.SetBech32PrefixForValidator(validatorAddressPrefix, validatorPubKeyPrefix)
	config.SetBech32PrefixForConsensusNode(consNodeAddressPrefix, consNodePubKeyPrefix)
	config.Seal()
}

func init() {
	// Set prefixes
	setPrefixes(app.AccountAddressPrefix)
}

var DefaultConsensusParams = &tmproto.ConsensusParams{
	Block: &tmproto.BlockParams{
		MaxBytes: 200000,
		MaxGas:   200000000,
	},
	Evidence: &tmproto.EvidenceParams{
		MaxAgeNumBlocks: 302400,
		MaxAgeDuration:  504 * time.Hour, // 3 weeks is the max duration
		MaxBytes:        10000,
	},
	Validator: &tmproto.ValidatorParams{
		PubKeyTypes: []string{
			tmtypes.ABCIPubKeyTypeEd25519,
		},
	},
}

type PeptideAppOptions struct {
	DB                  tmdb.DB
	HomePath            string
	ChainID             string
	IAVLDisableFastNode bool
	IAVLLazyLoading     bool
}

func New(chainID string, dir string, db tmdb.DB, logger tmlog.Logger) *PeptideApp {
	return NewWithOptions(PeptideAppOptions{
		ChainID:  chainID,
		HomePath: dir,
		DB:       db,
	}, logger)
}

// New creates application instance with in-memory database and disabled logging.
func NewWithOptions(options PeptideAppOptions, logger tmlog.Logger) *PeptideApp {
	logger.Info("new app with options",
		"chain_id", options.ChainID,
		"home_path", options.HomePath,
		"iavl_lazy_loading", options.IAVLLazyLoading,
		"iavl_disable_fast_node", options.IAVLDisableFastNode,
	)
	encoding := app.MakeEncodingConfig()
	mainApp := app.New(
		logger,
		options.DB,
		nil,
		true,
		map[int64]bool{},
		options.HomePath,
		0,
		encoding,
		simtestutil.EmptyAppOptions{},
		baseapp.SetChainID(options.ChainID),
		baseapp.SetIAVLDisableFastNode(options.IAVLDisableFastNode),
		baseapp.SetIAVLLazyLoading(options.IAVLLazyLoading),
	)

	newPeptideApp := &PeptideApp{
		App:                  mainApp,
		ValSet:               &tmtypes.ValidatorSet{},
		EncodingConfig:       &encoding,
		ChainId:              options.ChainID,
		BondDenom:            sdk.DefaultBondDenom,
		VotingPowerReduction: sdk.DefaultPowerReduction,
	}

	return newPeptideApp
}

// ImportAppStateAndValidators imports the application state, init height, and validators from ExportedApp defined by
// the chain App
func (a *PeptideApp) ImportAppStateAndValidators(
	exported *servertypes.ExportedApp,
) abci.ResponseInitChain {
	resp := a.InitChainWithGenesisStateAndHeight(exported.AppState, exported.Height)
	// iterate over exported.Validators
	tmValidators := make([]*tmtypes.Validator, len(exported.Validators))
	for i, val := range exported.Validators {
		tmValidators[i] = tmtypes.NewValidator(val.PubKey, val.Power)
	}
	a.ValSet = tmtypes.NewValidatorSet(tmValidators)
	// set consensusParam?
	return resp
}

func (a *PeptideApp) InitChainWithEmptyGenesis() abci.ResponseInitChain {
	genState := a.SimpleGenesis(NewValidatorSignerAccounts(1, 0), NewValidatorSignerAccounts(1, 0))
	return a.InitChainWithGenesisState(genState)
}

func (a *PeptideApp) InitChainWithGenesisState(state app.GenesisState) abci.ResponseInitChain {
	stateBytes := lo.Must(json.MarshalIndent(state, "", " "))
	req := &abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		Time:            time.Now(),
		AppStateBytes:   stateBytes,
	}
	// InitChain updates deliverState which is required when app.NewContext is called
	return a.InitChain(*req)
}

func (a *PeptideApp) InitChainWithGenesisStateAndHeight(state []byte, height int64) abci.ResponseInitChain {
	req := &abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		AppStateBytes:   state,
		Time:            time.Now(),
		InitialHeight:   height,
	}
	return a.InitChain(*req)
}

// This is what initiates the chain app initialisation. It's only meant to be called when the genesis is
// being sealed so the genesis block can be produced.
// - It triggers a call into the base app's InitChain()
// - Commits the app state to disk so it can be persisted across executions
// - Returns a "genesis header" with the genesis block height and app state hash
func (a *PeptideApp) Init(appState []byte, initialHeight int64, genesisTime time.Time) *tmproto.Header {
	response := a.InitChain(abci.RequestInitChain{
		ChainId:         a.ChainId,
		ConsensusParams: DefaultConsensusParams,
		AppStateBytes:   appState,
		Time:            genesisTime,
		InitialHeight:   initialHeight,
	})

	// this will store the app state into disk. Failing to call this will result in missing data the next
	// time the app is called
	a.Commit()

	// use LastBlockHeight() since it might not be the same as InitialHeight.
	return &tmproto.Header{
		Height:             a.LastBlockHeight(),
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
		Time:               genesisTime,
		AppHash:            response.AppHash,
	}
}

// TODO get rid of all this nonsense
func (a *PeptideApp) initValSetFromGenesis(genesisStateBytes []byte) error {
	var genesisState app.GenesisState
	if err := json.Unmarshal(genesisStateBytes, &genesisState); err != nil {
		return err
	}

	var stakingGenesis stakingtypes.GenesisState
	a.AppCodec().MustUnmarshalJSON(genesisState[stakingtypes.ModuleName], &stakingGenesis)

	valset, err := teststaking.ToTmValidators(stakingGenesis.Validators, a.VotingPowerReduction)
	if err != nil {
		return err
	}

	a.ValSet = tmtypes.NewValidatorSet(valset)
	return nil
}

// Resume the normal activity after a (chain) restart. It sets the required pointers according to the
// last known header (that comes from the block store) and calls into the base app's BeginBlock()
func (a *PeptideApp) Resume(lastHeader *tmproto.Header, genesisState []byte) error {
	a.lastHeader = lastHeader
	a.currentHeader = &tmproto.Header{
		Height:             a.LastBlockHeight() + 1,
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
	}

	if err := a.initValSetFromGenesis(genesisState); err != nil {
		return err
	}

	a.BeginBlock(abci.RequestBeginBlock{Header: *a.CurrentHeader()})
	return nil
}

// Rolls back the app state (i.e. commit multi store from the base app) to the specified height (version)
// If successful, the latest committed version is that of "height"
func (a *PeptideApp) RollbackToHeight(height int64) error {
	cms := a.CommitMultiStore()
	return cms.RollbackToVersion(height)
}

// DefaultGenesis create a default GenesisState, which is a map
// with module name as keys and JSON-marshaled genesisState per module as values
func (a *PeptideApp) DefaultGenesis() app.GenesisState {
	return app.NewDefaultGenesisState(a.AppCodec())
}

// SimpleGenesis creates a genesis with given genesis accounts `genAccs` and one derived validator.
// The first genesis account is used for delegation with bond amount of 1 voting power.
//
// `balances` should include staking tokens for all transactor accounts in `genAccs`.
// If left empty, `a.VotingPowerReduction` ie. 1 voting power worth staking tokens will be added for each account.
//
// Validators are saved in `a.ValSet` for block production later.
func (a *PeptideApp) SimpleGenesis(
	genAccs SignerAccounts,
	valAccs SignerAccounts,
	balances ...banktypes.Balance,
) app.GenesisState {
	if len(balances) == 0 {
		balances = genAccs.NewBalances(a.BondDenom, a.VotingPowerReduction)
	}
	genesisState := a.MultiDelegationGenesis(
		genAccs.GetGenesisAccounts(),
		balances,
		genAccs,
		valAccs,
		ValidatorSetDelegation{{{0, a.VotingPowerReduction}}},
	)
	return genesisState
}

// MultiDelegationGenesis creates a genesis with given genesis accounts `genAccs` and validators,
// and delegations.
// Each validator must be delegated with staking tokens ≥ 1 voting power, ie. a.ReductionVotingPower x 1.
// `delegations` must be the same length as `validators`, where its index is for validators.
func (a *PeptideApp) MultiDelegationGenesis(
	genAccs []GenesisAccount,
	balances []banktypes.Balance,
	delegators SignerAccounts,
	validators SignerAccounts,
	delegations ValidatorSetDelegation,
) app.GenesisState {
	genesisState := a.DefaultGenesis()
	//
	// set x/auth genesis
	//
	authGenesis := authtypes.NewGenesisState(authtypes.DefaultParams(), genAccs)
	genesisState[authtypes.ModuleName] = a.AppCodec().MustMarshalJSON(authGenesis)

	stakingValidators, stakingDelegations, totalValidatorSetBond := newStakingValidators(
		validators, delegators, delegations,
	)

	// bond pool holds all the bonded staking tokens
	bondPoolBalance := banktypes.Balance{
		Address: authtypes.NewModuleAddress(stakingtypes.BondedPoolName).String(),
		Coins: sdk.Coins{
			sdk.NewCoin(a.BondDenom, totalValidatorSetBond),
		},
	}

	totalBalances := append(balances, bondPoolBalance)
	// update total supply
	totalSupply := sdk.NewCoins()
	for _, b := range totalBalances {
		totalSupply = totalSupply.Add(b.Coins...)
	}

	//
	// set x/staking genesis
	//
	stakingGenesis := stakingtypes.NewGenesisState(stakingtypes.DefaultParams(), stakingValidators, stakingDelegations)
	genesisState[stakingtypes.ModuleName] = a.AppCodec().MustMarshalJSON(stakingGenesis)

	//
	// set x/bank genesis
	//
	bankGenesis := banktypes.NewGenesisState(
		banktypes.DefaultGenesisState().Params,
		totalBalances,
		totalSupply,
		[]banktypes.Metadata{},
		[]banktypes.SendEnabled{},
	)
	genesisState[banktypes.ModuleName] = a.AppCodec().MustMarshalJSON(bankGenesis)

	// Set app validators for block production
	a.ValSet = tmtypes.NewValidatorSet(
		lo.Must(teststaking.ToTmValidators(stakingValidators, a.VotingPowerReduction)),
	)
	return genesisState
}

// Clone returns a new PeptideApp with genesis state from the current PeptideApp.
// The new app will have the same validator set `ValSet` as the current app.
// Note the new PeptideApp is not committed yet. Call CommitAndBeginBlock to commit.
//
// Useful for testing module genesis export/import.
func (a *PeptideApp) Clone() *PeptideApp {
	cloned := New(a.ChainId, "/tmp/monomer-PeptideApp", tmdb.NewMemDB(), a.Logger())
	cloned.ValSet = a.ValSet.Copy()
	cloned.InitChainWithGenesisState(a.ExportGenesis())
	return cloned
}

// ExportGenesis returns a copy of the app's genesis state
func (a *PeptideApp) ExportGenesis() app.GenesisState {
	exportedApp := lo.Must(a.ExportAppStateAndValidators(false, nil, nil))
	var genesisState app.GenesisState
	lo.Must0(tmjson.Unmarshal(exportedApp.AppState, &genesisState))
	return genesisState
}

// Commit pending changes to chain state and start a new block.
// Will error if there is no deliverState, eg. InitChain is not called before first block.
func (a *PeptideApp) CommitAndBeginNextBlock(timestamp eth.Uint64Quantity) *PeptideApp {
	a.Commit()
	a.OnCommit(timestamp)

	a.BeginBlock(abci.RequestBeginBlock{Header: *a.CurrentHeader()})
	return a
}

// OnCommit updates the last header and current header after App Commit or InitChain
func (a *PeptideApp) OnCommit(timestamp eth.Uint64Quantity) {
	// update last header to the committed time and app hash
	lastHeader := a.currentHeader
	lastHeader.Time = time.Unix(int64(timestamp), 0)
	lastHeader.AppHash = a.LastCommitID().Hash
	a.lastHeader = lastHeader

	// start a new partial header for next round
	a.currentHeader = &tmproto.Header{
		Height:             a.LastBlockHeight() + 1,
		ValidatorsHash:     a.ValSet.Hash(),
		NextValidatorsHash: a.ValSet.Hash(),
		ChainID:            a.ChainId,
		Time:               time.Unix(int64(timestamp), 0),
	}
}

// CurrentHeader is the header that is being built, which is not committed yet
func (a *PeptideApp) CurrentHeader() *tmproto.Header {
	return a.currentHeader
}

// LastHeader is the header that was committed, either as a genesis block header or the latest committed block header
func (a *PeptideApp) LastHeader() *tmproto.Header {
	return a.lastHeader
}

// Return a Cosmos-SDK context
func (a *PeptideApp) NewUncachedSdkContext() sdk.Context {
	return a.NewUncachedContext(false, *a.LastHeader())
}

// Return a SDK context wrapped in Go context, which is required by GRPC handler
func (a *PeptideApp) NewWrappedUncachedContext() context.Context {
	return a.WrapSDKContext(a.NewUncachedSdkContext())
}

// Return a SDK context wrapped in Go context, which is required by GRPC handler
func (a *PeptideApp) NewUncachedContextWithHeight(height int64) context.Context {
	return a.WrapSDKContext(a.NewUncachedSdkContext().WithBlockHeight(height))
}

// Convert a SDK context to Go context
func (a *PeptideApp) WrapSDKContext(ctx sdk.Context) context.Context {
	return sdk.WrapSDKContext(ctx)
}

// SignMsgs signs a list of Msgs `msg` with `signers`
func (a *PeptideApp) SignMsgs(signers []*SignerAccount, msg ...sdk.Msg) (sdk.Tx, error) {
	tx, err := GenTx(a.EncodingConfig.TxConfig, msg,
		sdk.Coins{sdk.NewInt64Coin(a.BondDenom, 0)},
		DefaultGenTxGas,
		a.ChainId,
		nil,
		signers...,
	)
	return tx, err
}

// SignAndDeliverMsgs signs a list of Msgs `msg` with a single `signer`, and delivers it to the chain app
func (a *PeptideApp) SignAndDeliverMsgs(
	signer *SignerAccount,
	msgs ...sdk.Msg,
) (sdk.Tx, *sdk.Result, *sdk.GasInfo, error) {
	return a.MultiSignAndDeliverMsgs([]*SignerAccount{signer}, msgs...)
}

// MultiSignAndDeliverMsgs signs  a list of Msgs `msg` with multiple `signers`, and delivers it to the chain app
func (a *PeptideApp) MultiSignAndDeliverMsgs(
	signers []*SignerAccount,
	msgs ...sdk.Msg,
) (sdk.Tx, *sdk.Result, *sdk.GasInfo, error) {
	tx, err := GenTx(a.EncodingConfig.TxConfig, msgs,
		sdk.Coins{sdk.NewInt64Coin(a.BondDenom, 0)},
		DefaultGenTxGas,
		a.ChainId,
		nil,
		signers...,
	)
	if err != nil {
		return nil, nil, nil, err
	}

	gasInfo, result, err := a.SimDeliver(a.EncodingConfig.TxConfig.TxEncoder(), tx)
	// Increment account sequences regardless of tx execution result to simplify testing In some cases, tx execution may
	// fail but account sequence is still incremented; while in other cases, account sequence is not incremented when tx fails
	for _, signer := range signers {
		signer.SetSequence(signer.GetSequence() + 1)
		a.AccountKeeper.SetAccount(a.NewUncachedSdkContext(), signer.GenesisAccount)
	}
	return tx, result, &gasInfo, err
}

// Return account from committed state
func (a *PeptideApp) GetAccount(addr sdk.AccAddress) AccountI {
	return a.AccountKeeper.GetAccount(a.NewUncachedSdkContext(), addr)
}

func (a *PeptideApp) ReportMetrics() {
	// TODO
}
