package keeper_test

import (
	"context"
	"testing"

	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptestutil "github.com/polymerdao/monomer/x/rollup/testutil"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type KeeperTestSuite struct {
	suite.Suite
	ctx          context.Context
	rollupKeeper *keeper.Keeper
	bankKeeper   *rolluptestutil.MockBankKeeper
	rollupStore  storetypes.KVStore
	eventManger  sdk.EventManagerI
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (s *KeeperTestSuite) SetupSubTest() {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	s.ctx = testutil.DefaultContextWithDB(
		s.T(),
		storeKey,
		storetypes.NewTransientStoreKey("transient_test")).Ctx
	s.bankKeeper = rolluptestutil.NewMockBankKeeper(gomock.NewController(s.T()))
	s.rollupKeeper = keeper.NewKeeper(
		moduletestutil.MakeTestEncodingConfig().Codec,
		runtime.NewKVStoreService(storeKey),
		s.bankKeeper,
	)
	sdkCtx := sdk.UnwrapSDKContext(s.ctx)
	s.rollupStore = sdkCtx.KVStore(storeKey)
	s.eventManger = sdkCtx.EventManager()
}

func (s *KeeperTestSuite) mockBurnETH() {
	s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), gomock.Any(), types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
	s.bankKeeper.EXPECT().BurnCoins(gomock.Any(), types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
}

func (s *KeeperTestSuite) mockMintETH() {
	s.bankKeeper.EXPECT().MintCoins(gomock.Any(), types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}
