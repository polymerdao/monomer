package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	"github.com/polymerdao/monomer/x/rollup/keeper"
	rolluptestutil "github.com/polymerdao/monomer/x/rollup/testutil"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type KeeperTestSuite struct {
	suite.Suite
	ctx           context.Context
	rollupKeeper  *keeper.Keeper
	bankKeeper    *rolluptestutil.MockBankKeeper
	accountKeeper *rolluptestutil.MockAccountKeeper
	rollupStore   storetypes.KVStore
	eventManger   sdk.EventManagerI
}

func TestKeeperTestSuite(t *testing.T) {
	suite.Run(t, new(KeeperTestSuite))
}

func (s *KeeperTestSuite) SetupTest() {
	s.setup()
}

func (s *KeeperTestSuite) SetupSubTest() {
	s.setup()
}

func (s *KeeperTestSuite) setup() {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	s.ctx = testutil.DefaultContextWithDB(
		s.T(),
		storeKey,
		storetypes.NewTransientStoreKey("transient_test")).Ctx
	s.accountKeeper = rolluptestutil.NewMockAccountKeeper(gomock.NewController(s.T()))
	s.bankKeeper = rolluptestutil.NewMockBankKeeper(gomock.NewController(s.T()))
	s.rollupKeeper = keeper.NewKeeper(
		moduletestutil.MakeTestEncodingConfig().Codec,
		runtime.NewKVStoreService(storeKey),
		authtypes.NewModuleAddress(govtypes.ModuleName),
		s.bankKeeper,
		s.accountKeeper,
	)
	sdkCtx := sdk.UnwrapSDKContext(s.ctx)
	s.rollupStore = sdkCtx.KVStore(storeKey)
	s.eventManger = sdkCtx.EventManager()
}

func (s *KeeperTestSuite) mockBurn() {
	s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(s.ctx, gomock.Any(), types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
	s.bankKeeper.EXPECT().BurnCoins(s.ctx, types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
}

func (s *KeeperTestSuite) mockMint() {
	s.bankKeeper.EXPECT().MintCoins(s.ctx, types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
	s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(s.ctx, types.ModuleName, gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}

func (s *KeeperTestSuite) mockFeeCollector() {
	mockFeeCollectorAddress := sdk.AccAddress("fee_collector")
	s.accountKeeper.EXPECT().GetModuleAddress(authtypes.FeeCollectorName).Return(mockFeeCollectorAddress).AnyTimes()
	s.bankKeeper.EXPECT().GetBalance(s.ctx, mockFeeCollectorAddress, types.WEI).Return(sdk.NewCoin(types.WEI, math.NewInt(1_000_000))).AnyTimes()
	s.bankKeeper.EXPECT().SendCoinsFromModuleToModule(s.ctx, authtypes.FeeCollectorName, types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
	s.bankKeeper.EXPECT().BurnCoins(s.ctx, types.ModuleName, gomock.Any()).Return(nil).AnyTimes()
}
