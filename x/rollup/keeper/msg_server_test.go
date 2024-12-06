package keeper_test

import (
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/mock/gomock"
	"github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/x/rollup/types"
)

func (s *KeeperTestSuite) TestSetL1Attributes() {
	l1AttributesTx, _, _ := testutils.GenerateEthTxs(s.T())
	l1AttributesTxBz := testutils.TxToBytes(s.T(), l1AttributesTx)
	_, err := s.rollupKeeper.SetL1Attributes(s.ctx, &types.MsgSetL1Attributes{
		L1BlockInfo: &types.L1BlockInfo{
			Number: 1,
		},
		EthTx: l1AttributesTxBz,
	})
	s.NoError(err)

	l1BlockInfoBz := s.rollupStore.Get([]byte(types.L1BlockInfoKey))
	s.Require().NotNil(l1BlockInfoBz)

	l1BlockInfo := &types.L1BlockInfo{}
	err = l1BlockInfo.Unmarshal(l1BlockInfoBz)
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), l1BlockInfo.Number)
}

func (s *KeeperTestSuite) TestApplyUserDeposit() {
	l1AttributesTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(s.T())
	// The only constraint for a contract creation tx is that it must be a non-system DepositTx with no To field
	contractCreationTx := gethtypes.NewTx(&gethtypes.DepositTx{})

	l1AttributesTxBz := testutils.TxToBytes(s.T(), l1AttributesTx)
	depositTxBz := testutils.TxToBytes(s.T(), depositTx)
	cosmosEthTxBz := testutils.TxToBytes(s.T(), cosmosEthTx)
	contractCreationTxBz := testutils.TxToBytes(s.T(), contractCreationTx)
	invalidTxBz := []byte("invalid tx bytes")

	_, err := s.rollupKeeper.SetL1Attributes(s.ctx, &types.MsgSetL1Attributes{
		L1BlockInfo: &types.L1BlockInfo{
			Time: uint64(time.Now().Unix()),
		},
		EthTx: l1AttributesTxBz,
	})
	s.NoError(err)

	tests := map[string]struct {
		txBytes            []byte
		setupMocks         func()
		shouldError        bool
		expectedEventTypes []string
	}{
		"successful message with single user deposit tx": {
			txBytes:     depositTxBz,
			shouldError: false,
			expectedEventTypes: []string{
				sdk.EventTypeMessage,
				types.EventTypeMintETH,
			},
		},
		"invalid deposit tx bytes": {
			txBytes:     invalidTxBz,
			shouldError: true,
		},
		"non-deposit tx passed in as user deposit tx": {
			txBytes:     cosmosEthTxBz,
			shouldError: true,
		},
		"l1 attributes tx passed in as user deposit tx": {
			txBytes:     l1AttributesTxBz,
			shouldError: true,
		},
		"contract creation tx passed in as user deposit tx": {
			txBytes:     contractCreationTxBz,
			shouldError: true,
		},
		"bank keeper mint coins failure": {
			txBytes: depositTxBz,
			setupMocks: func() {
				s.bankKeeper.EXPECT().MintCoins(s.ctx, types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnauthorized)
			},
			shouldError: true,
		},
		"bank keeper send coins failure": {
			txBytes: depositTxBz,
			setupMocks: func() {
				s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(s.ctx, types.ModuleName, gomock.Any(), gomock.Any()).Return(sdkerrors.ErrUnknownRequest)
			},
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			if test.setupMocks != nil {
				test.setupMocks()
			}
			s.mockMint()

			resp, err := s.rollupKeeper.ApplyUserDeposit(s.ctx, &types.MsgApplyUserDeposit{
				Tx: test.txBytes,
			})

			if test.shouldError {
				s.Require().Error(err)
				s.Require().Nil(resp)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(resp)

				// Verify that the expected event types are emitted
				for i, event := range s.eventManger.Events() {
					s.Require().Equal(test.expectedEventTypes[i], event.Type)
				}
			}
		})
	}
}

func (s *KeeperTestSuite) TestInitiateWithdrawal() {
	sender := sdk.AccAddress("addr").String()
	l1Target := "0x12345abcde"
	withdrawalAmount := math.NewInt(1000000)

	//nolint:dupl
	tests := map[string]struct {
		sender      string
		setupMocks  func()
		shouldError bool
	}{
		"successful message": {
			sender:      sender,
			shouldError: false,
		},
		"invalid sender addr": {
			sender:      "invalid",
			shouldError: true,
		},
		"bank keeper insufficient funds failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(s.ctx, gomock.Any(), types.ModuleName, gomock.Any()).Return(types.ErrBurnETH).AnyTimes()
			},
			sender:      sender,
			shouldError: true,
		},
		"bank keeper burn coins failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().BurnCoins(s.ctx, types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest).AnyTimes()
			},
			sender:      sender,
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			if test.setupMocks != nil {
				test.setupMocks()
			}
			s.mockBurn()

			resp, err := s.rollupKeeper.InitiateWithdrawal(s.ctx, &types.MsgInitiateWithdrawal{
				Sender: test.sender,
				Target: l1Target,
				Value:  withdrawalAmount,
			})

			if test.shouldError {
				s.Require().Error(err)
				s.Require().Nil(resp)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(resp)

				// Verify that the expected event types are emitted
				expectedEventTypes := []string{
					sdk.EventTypeMessage,
					types.EventTypeWithdrawalInitiated,
					types.EventTypeBurnETH,
				}
				for i, event := range s.eventManger.Events() {
					s.Require().Equal(expectedEventTypes[i], event.Type)
				}
			}
		})
	}
}

func (s *KeeperTestSuite) TestInitiateERC20Withdrawal() {
	sender := sdk.AccAddress("addr").String()
	l1Target := "0x12345abcde"
	erc20TokenAddress := "0x0123456789abcdef"
	withdrawalAmount := math.NewInt(1000000)

	//nolint:dupl
	tests := map[string]struct {
		sender      string
		setupMocks  func()
		shouldError bool
	}{
		"successful message": {
			sender:      sender,
			shouldError: false,
		},
		"invalid sender addr": {
			sender:      "invalid",
			shouldError: true,
		},
		"bank keeper insufficient funds failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(s.ctx, gomock.Any(), types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest).AnyTimes()
			},
			sender:      sender,
			shouldError: true,
		},
		"bank keeper burn coins failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().BurnCoins(s.ctx, types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest).AnyTimes()
			},
			sender:      sender,
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			if test.setupMocks != nil {
				test.setupMocks()
			}
			s.mockBurn()

			resp, err := s.rollupKeeper.InitiateERC20Withdrawal(s.ctx, &types.MsgInitiateERC20Withdrawal{
				Sender:       test.sender,
				Target:       l1Target,
				TokenAddress: erc20TokenAddress,
				Value:        withdrawalAmount,
			})

			if test.shouldError {
				s.Require().Error(err)
				s.Require().Nil(resp)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(resp)

				// Verify that the expected event types are emitted
				expectedEventTypes := []string{
					sdk.EventTypeMessage,
					types.EventTypeWithdrawalInitiated,
					types.EventTypeBurnERC20,
				}
				for i, event := range s.eventManger.Events() {
					s.Require().Equal(expectedEventTypes[i], event.Type)
				}
			}
		})
	}
}

func (s *KeeperTestSuite) TestInitiateFeeWithdrawal() {
	tests := map[string]struct {
		setupMocks  func()
		shouldError bool
	}{
		"successful message": {
			shouldError: false,
		},
		"fee collector address not found": {
			setupMocks: func() {
				s.accountKeeper.EXPECT().GetModuleAddress(authtypes.FeeCollectorName).Return(nil)
			},
			shouldError: true,
		},
		"fee collector balance below minimum withdrawal amount": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().GetBalance(s.ctx, gomock.Any(), types.WEI).Return(sdk.NewCoin(types.WEI, math.NewInt(1)))
			},
			shouldError: true,
		},
		"bank keeper send coins failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().SendCoinsFromModuleToModule(s.ctx, authtypes.FeeCollectorName, types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest)
			},
			shouldError: true,
		},
		"bank keeper burn coins failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().BurnCoins(s.ctx, types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest)
			},
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			if test.setupMocks != nil {
				test.setupMocks()
			}
			s.mockFeeCollector()

			params := types.DefaultParams()
			err := s.rollupKeeper.SetParams(sdk.UnwrapSDKContext(s.ctx), &params)
			s.Require().NoError(err)

			resp, err := s.rollupKeeper.InitiateFeeWithdrawal(s.ctx, &types.MsgInitiateFeeWithdrawal{
				Sender: sdk.AccAddress("addr").String(),
			})

			if test.shouldError {
				s.Require().Error(err)
				s.Require().Nil(resp)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(resp)

				// Verify that the expected event types are emitted
				expectedEventTypes := []string{
					sdk.EventTypeMessage,
					types.EventTypeWithdrawalInitiated,
					types.EventTypeBurnETH,
				}
				for i, event := range s.eventManger.Events() {
					s.Require().Equal(expectedEventTypes[i], event.Type)
				}
			}
		})
	}
}

func (s *KeeperTestSuite) TestUpdateParams() {
	authority := authtypes.NewModuleAddress(govtypes.ModuleName)
	validParams := types.DefaultParams()
	invalidParams := types.Params{}

	tests := map[string]struct {
		authority   sdk.AccAddress
		params      types.Params
		shouldError bool
	}{
		"valid authority with valid params": {
			authority:   authority,
			params:      validParams,
			shouldError: false,
		},
		"invalid authority": {
			authority:   sdk.AccAddress("invalid_authority"),
			params:      validParams,
			shouldError: true,
		},
		"valid authority with invalid params": {
			authority:   authority,
			params:      invalidParams,
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			resp, err := s.rollupKeeper.UpdateParams(s.ctx, &types.MsgUpdateParams{
				Authority: test.authority.String(),
				Params:    test.params,
			})

			if test.shouldError {
				s.Require().Error(err)
				s.Require().Nil(resp)
			} else {
				s.Require().NoError(err)
				s.Require().NotNil(resp)

				params, err := s.rollupKeeper.GetParams(sdk.UnwrapSDKContext(s.ctx))
				s.Require().NoError(err)
				s.Require().Equal(test.params, *params)
			}
		})
	}
}
