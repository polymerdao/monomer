package integration_test

import (
	"math/big"
	"testing"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil/integration"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/cosmos/cosmos-sdk/x/auth"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authsims "github.com/cosmos/cosmos-sdk/x/auth/simulation"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/bank"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/common"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	monomertestutils "github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/utils"
	"github.com/polymerdao/monomer/x/rollup"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

func TestRollup(t *testing.T) {
	integrationApp := setupIntegrationApp(t)
	queryClient := banktypes.NewQueryClient(integrationApp.QueryHelper())

	monomerSigner := "cosmos1fl48vsnmsdzcv85q5d2q4z5ajdha8yu34mf0eh"
	erc20tokenAddr := common.HexToAddress("0xabcdef123456")
	erc20userAddr := common.HexToAddress("0x123456abcdef")
	erc20depositAmount := big.NewInt(100)

	l1AttributesTx, depositTx, _ := monomertestutils.GenerateEthTxs(t)
	erc20DepositTx := monomertestutils.GenerateERC20DepositTx(t, erc20tokenAddr, erc20userAddr, erc20depositAmount)
	l1WithdrawalAddr := common.HexToAddress("0x112233445566").String()

	l1AttributesTxBz := monomertestutils.TxToBytes(t, l1AttributesTx)
	depositTxBz := monomertestutils.TxToBytes(t, depositTx)
	erc20DepositTxBz := monomertestutils.TxToBytes(t, erc20DepositTx)

	depositAmount := depositTx.Mint()
	from, err := gethtypes.NewCancunSigner(depositTx.ChainId()).Sender(depositTx)
	require.NoError(t, err)
	userAddr := utils.EvmToCosmosAddress(from)

	// query the user's ETH balance and assert it's zero
	require.Equal(t, math.ZeroInt(), queryUserETHBalance(t, queryClient, userAddr, integrationApp))

	// query the user's ERC20 balance and assert it's zero
	require.Equal(t, math.ZeroInt(), queryUserERC20Balance(t, queryClient, utils.EvmToCosmosAddress(erc20userAddr), erc20tokenAddr, integrationApp))

	// send an invalid MsgApplyL1Txs and assert error
	_, err = integrationApp.RunMsg(&rolluptypes.MsgApplyL1Txs{
		TxBytes:     [][]byte{l1AttributesTxBz, l1AttributesTxBz},
		FromAddress: monomerSigner,
	})
	require.Error(t, err)

	// send a successful MsgApplyL1Txs and mint ETH to user
	_, err = integrationApp.RunMsg(&rolluptypes.MsgApplyL1Txs{
		TxBytes:     [][]byte{l1AttributesTxBz, depositTxBz, erc20DepositTxBz},
		FromAddress: monomerSigner,
	})
	require.NoError(t, err)

	// query the user's ETH balance and assert it's equal to the deposit amount
	require.Equal(t, depositAmount, queryUserETHBalance(t, queryClient, userAddr, integrationApp).BigInt())

	// query the user's ERC20 balance and assert it's equal to the deposit amount
	require.Equal(t, erc20depositAmount, queryUserERC20Balance(t, queryClient, utils.EvmToCosmosAddress(erc20userAddr), erc20tokenAddr, integrationApp).BigInt())

	// try to withdraw more than deposited and assert error
	_, err = integrationApp.RunMsg(&rolluptypes.MsgInitiateWithdrawal{
		Sender:   userAddr.String(),
		Target:   l1WithdrawalAddr,
		Value:    math.NewIntFromBigInt(depositAmount).Add(math.OneInt()),
		GasLimit: new(big.Int).SetUint64(100_000_000).Bytes(),
		Data:     []byte{0x01, 0x02, 0x03},
	})
	require.Error(t, err)

	// send a successful MsgInitiateWithdrawal
	_, err = integrationApp.RunMsg(&rolluptypes.MsgInitiateWithdrawal{
		Sender:   userAddr.String(),
		Target:   l1WithdrawalAddr,
		Value:    math.NewIntFromBigInt(depositAmount),
		GasLimit: new(big.Int).SetUint64(100_000_000).Bytes(),
		Data:     []byte{0x01, 0x02, 0x03},
	})
	require.NoError(t, err)

	// query the user's ETH balance and assert it's zero
	require.Equal(t, math.ZeroInt(), queryUserETHBalance(t, queryClient, userAddr, integrationApp))
}

func setupIntegrationApp(t *testing.T) *integration.App {
	encodingCfg := moduletestutil.MakeTestEncodingConfig(auth.AppModuleBasic{}, bank.AppModuleBasic{}, rollup.AppModuleBasic{})
	keys := storetypes.NewKVStoreKeys(authtypes.StoreKey, banktypes.StoreKey, rolluptypes.StoreKey)
	authority := authtypes.NewModuleAddress("gov").String()

	logger := log.NewTestLogger(t)

	cms := integration.CreateMultiStore(keys, logger)

	accountKeeper := authkeeper.NewAccountKeeper(
		encodingCfg.Codec,
		runtime.NewKVStoreService(keys[authtypes.StoreKey]),
		authtypes.ProtoBaseAccount,
		map[string][]string{rolluptypes.ModuleName: {authtypes.Minter, authtypes.Burner}},
		addresscodec.NewBech32Codec("cosmos"),
		"cosmos",
		authority,
	)
	bankKeeper := bankkeeper.NewBaseKeeper(
		encodingCfg.Codec,
		runtime.NewKVStoreService(keys[banktypes.StoreKey]),
		accountKeeper,
		map[string]bool{},
		authority,
		logger,
	)
	rollupKeeper := rollupkeeper.NewKeeper(
		encodingCfg.Codec,
		runtime.NewKVStoreService(keys[rolluptypes.StoreKey]),
		bankKeeper,
	)

	authModule := auth.NewAppModule(encodingCfg.Codec, accountKeeper, authsims.RandomGenesisAccounts, nil)
	bankModule := bank.NewAppModule(encodingCfg.Codec, bankKeeper, accountKeeper, nil)
	rollupModule := rollup.NewAppModule(encodingCfg.Codec, rollupKeeper)

	integrationApp := integration.NewIntegrationApp(
		sdk.NewContext(cms, cmtproto.Header{}, false, logger),
		logger,
		keys,
		encodingCfg.Codec,
		map[string]appmodule.AppModule{
			authtypes.ModuleName:   authModule,
			banktypes.ModuleName:   bankModule,
			rolluptypes.ModuleName: rollupModule,
		},
	)
	rolluptypes.RegisterMsgServer(integrationApp.MsgServiceRouter(), rollupKeeper)
	banktypes.RegisterQueryServer(integrationApp.QueryHelper(), bankkeeper.NewQuerier(&bankKeeper))

	return integrationApp
}

func queryUserBalance(t *testing.T, queryClient banktypes.QueryClient, userAddr sdk.AccAddress, denom string, app *integration.App) math.Int {
	resp, err := queryClient.Balance(app.Context(), &banktypes.QueryBalanceRequest{
		Address: userAddr.String(),
		Denom:   denom,
	})
	require.NoError(t, err)
	return resp.Balance.Amount
}

func queryUserETHBalance(t *testing.T, queryClient banktypes.QueryClient, userAddr sdk.AccAddress, app *integration.App) math.Int {
	return queryUserBalance(t, queryClient, userAddr, rolluptypes.ETH, app)
}

func queryUserERC20Balance(t *testing.T, queryClient banktypes.QueryClient, userAddr sdk.AccAddress, erc20addr common.Address, app *integration.App) math.Int {
	return queryUserBalance(t, queryClient, userAddr, "erc20/"+erc20addr.String()[2:], app)
}
