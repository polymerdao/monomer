package rollup_test

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
	rolluptestutils "github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/x/rollup"
	rollupkeeper "github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
)

func TestRollup(t *testing.T) {
	integrationApp, bankKeeper := setupIntegrationApp(t)

	monomerSigner := "cosmos1fl48vsnmsdzcv85q5d2q4z5ajdha8yu34mf0eh"
	l1AttributesTx, depositTx, _ := rolluptestutils.GenerateEthTxs(t)
	l1WithdrawalAddr := common.HexToAddress("0x12345").String()

	l1AttributesTxBz := rolluptestutils.TxToBytes(t, l1AttributesTx)
	depositTxBz := rolluptestutils.TxToBytes(t, depositTx)

	depositAmount := depositTx.Value()
	var userAddr sdk.AccAddress = depositTx.To().Bytes()

	// query the user's ETH balance and assert it's zero
	require.Equal(t, math.ZeroInt(), bankKeeper.GetBalance(integrationApp.Context(), userAddr, rolluptypes.ETH).Amount)

	// send an invalid MsgApplyL1Txs and assert error
	_, err := integrationApp.RunMsg(&rolluptypes.MsgApplyL1Txs{
		TxBytes:     [][]byte{l1AttributesTxBz, l1AttributesTxBz},
		FromAddress: monomerSigner,
	})
	require.Error(t, err)

	// send a successful MsgApplyL1Txs and mint ETH to user
	_, err = integrationApp.RunMsg(&rolluptypes.MsgApplyL1Txs{
		TxBytes:     [][]byte{l1AttributesTxBz, depositTxBz},
		FromAddress: monomerSigner,
	})
	require.NoError(t, err)

	// query the user's ETH balance and assert it's equal to the deposit amount
	require.Equal(t, depositAmount, bankKeeper.GetBalance(integrationApp.Context(), userAddr, rolluptypes.ETH).Amount.BigInt())

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
	require.Equal(t, math.ZeroInt(), bankKeeper.GetBalance(integrationApp.Context(), userAddr, rolluptypes.ETH).Amount)
}

func setupIntegrationApp(t *testing.T) (*integration.App, bankkeeper.BaseKeeper) {
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
	rolluptypes.RegisterMsgServer(integrationApp.MsgServiceRouter(), rollupkeeper.NewMsgServerImpl(rollupKeeper))

	return integrationApp, bankKeeper
}
