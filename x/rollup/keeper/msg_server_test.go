package keeper_test

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/mock/gomock"
	"github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/x/rollup/types"
)

func (s *KeeperTestSuite) TestApplyL1Txs() {
	l1AttributesTx, depositTx, cosmosEthTx := testutils.GenerateEthTxs(s.T())
	// The only constraint for a contract creation tx is that it must be a non-system DepositTx with no To field
	contractCreationTx := gethtypes.NewTx(&gethtypes.DepositTx{})

	l1AttributesTxBz := testutils.TxToBytes(s.T(), l1AttributesTx)
	depositTxBz := testutils.TxToBytes(s.T(), depositTx)
	cosmosEthTxBz := testutils.TxToBytes(s.T(), cosmosEthTx)
	contractCreationTxBz := testutils.TxToBytes(s.T(), contractCreationTx)
	invalidTxBz := []byte("invalid tx bytes")

	tests := map[string]struct {
		txBytes            [][]byte
		setupMocks         func()
		shouldError        bool
		expectedEventTypes []string
	}{
		"successful message with no user deposit txs": {
			txBytes:     [][]byte{l1AttributesTxBz},
			shouldError: false,
			expectedEventTypes: []string{
				sdk.EventTypeMessage,
			},
		},
		"successful message with single user deposit tx": {
			txBytes:     [][]byte{l1AttributesTxBz, depositTxBz},
			shouldError: false,
			expectedEventTypes: []string{
				sdk.EventTypeMessage,
				types.EventTypeMintETH,
			},
		},
		"successful message with multiple user deposit txs": {
			txBytes:     [][]byte{l1AttributesTxBz, depositTxBz, depositTxBz},
			shouldError: false,
			expectedEventTypes: []string{
				sdk.EventTypeMessage,
				types.EventTypeMintETH,
				types.EventTypeMintETH,
			},
		},
		"invalid l1 attributes tx bytes": {
			txBytes:     [][]byte{invalidTxBz, depositTxBz},
			shouldError: true,
		},
		"non-deposit tx passed in as l1 attributes tx": {
			txBytes:     [][]byte{cosmosEthTxBz, depositTxBz},
			shouldError: true,
		},
		"user deposit tx passed in as l1 attributes tx": {
			txBytes:     [][]byte{depositTxBz, depositTxBz},
			shouldError: true,
		},
		"invalid user deposit tx bytes": {
			txBytes:     [][]byte{l1AttributesTxBz, invalidTxBz},
			shouldError: true,
		},
		"non-deposit tx passed in as user deposit tx": {
			txBytes:     [][]byte{l1AttributesTxBz, cosmosEthTxBz},
			shouldError: true,
		},
		"l1 attributes tx passed in as user deposit tx": {
			txBytes:     [][]byte{l1AttributesTxBz, l1AttributesTxBz},
			shouldError: true,
		},
		"contract creation tx passed in as user deposit tx": {
			txBytes:     [][]byte{l1AttributesTxBz, contractCreationTxBz},
			shouldError: true,
		},
		"one valid l1 user deposit tx and an invalid tx passed in as user deposit txs": {
			txBytes:     [][]byte{l1AttributesTxBz, depositTxBz, invalidTxBz},
			shouldError: true,
		},
		"bank keeper mint coins failure": {
			txBytes: [][]byte{l1AttributesTxBz, depositTxBz},
			setupMocks: func() {
				s.bankKeeper.EXPECT().MintCoins(gomock.Any(), types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnauthorized)
			},
			shouldError: true,
		},
		"bank keeper send coins failure": {
			txBytes: [][]byte{l1AttributesTxBz, depositTxBz},
			setupMocks: func() {
				s.bankKeeper.EXPECT().SendCoinsFromModuleToAccount(gomock.Any(), types.ModuleName, gomock.Any(), gomock.Any()).Return(sdkerrors.ErrUnknownRequest)
			},
			shouldError: true,
		},
	}

	for name, test := range tests {
		s.Run(name, func() {
			if test.setupMocks != nil {
				test.setupMocks()
			}
			s.mockMintETH()

			resp, err := s.rollupKeeper.ApplyL1Txs(s.ctx, &types.MsgApplyL1Txs{
				TxBytes: test.txBytes,
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

				// Verify that the l1 block info and l1 block history are saved to the store
				expectedBlockInfo := eth.BlockToInfo(testutils.GenerateL1Block())

				l1BlockInfoBz := s.rollupStore.Get([]byte(types.KeyL1BlockInfo))
				s.Require().NotNil(l1BlockInfoBz)

				l1BlockInfo := &types.L1BlockInfo{}
				err = l1BlockInfo.Unmarshal(l1BlockInfoBz)
				s.Require().NoError(err)
				s.Require().Equal(expectedBlockInfo.NumberU64(), l1BlockInfo.Number)
				s.Require().Equal(expectedBlockInfo.BaseFee().Bytes(), l1BlockInfo.BaseFee)
				s.Require().Equal(expectedBlockInfo.Time(), l1BlockInfo.Time)
				s.Require().Equal(expectedBlockInfo.Hash().Bytes(), l1BlockInfo.BlockHash)
			}
		})
	}
}

func (s *KeeperTestSuite) TestInitiateWithdrawal() {
	sender := sdk.AccAddress("addr").String()
	l1Target := "0x12345abcde"
	withdrawalAmount := math.NewInt(1000000)

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
				s.bankKeeper.EXPECT().SendCoinsFromAccountToModule(gomock.Any(), gomock.Any(), types.ModuleName, gomock.Any()).Return(types.ErrBurnETH)
			},
			sender:      sender,
			shouldError: true,
		},
		"bank keeper burn coins failure": {
			setupMocks: func() {
				s.bankKeeper.EXPECT().BurnCoins(gomock.Any(), types.ModuleName, gomock.Any()).Return(sdkerrors.ErrUnknownRequest)
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
			s.mockBurnETH()

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
