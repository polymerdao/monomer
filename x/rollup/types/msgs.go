package types

import (
	"errors"
	"fmt"
	"math/big"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

const (
	MinTxGasLimit = params.TxGas
	MaxTxGasLimit = params.MaxGasLimit
)

var _ sdktypes.Msg = (*MsgApplyUserDeposit)(nil)

func (m *MsgApplyUserDeposit) ValidateBasic() error {
	if m.Tx == nil {
		return errors.New("tx is nil")
	}
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(m.Tx); err != nil {
		return fmt.Errorf("unmarshal binary deposit tx: %v", err)
	}
	if !tx.IsDepositTx() {
		return errors.New("tx is not a deposit tx")
	}
	if tx.IsSystemTx() {
		return errors.New("tx must not be a system tx")
	}
	return nil
}

func (*MsgApplyUserDeposit) Type() string {
	return "apply_user_deposit"
}

func (*MsgApplyUserDeposit) Route() string {
	return ModuleName
}

var _ sdktypes.Msg = (*MsgSetL1Attributes)(nil)

func (m *MsgSetL1Attributes) ValidateBasic() error {
	return nil
}

func (*MsgSetL1Attributes) Type() string {
	return "set_l1_attributes"
}

func (*MsgSetL1Attributes) Route() string {
	return ModuleName
}

var _ sdktypes.Msg = (*MsgInitiateWithdrawal)(nil)

func (m *MsgInitiateWithdrawal) ValidateBasic() error {
	// Check if the Ethereum address is valid
	if !common.IsHexAddress(m.Target) {
		return fmt.Errorf("invalid Ethereum address: %s", m.Target)
	}
	// Check if the gas limit is within the allowed range.
	gasLimit := new(big.Int).SetBytes(m.GasLimit).Uint64()
	if gasLimit < MinTxGasLimit || gasLimit > MaxTxGasLimit {
		return fmt.Errorf("gas limit must be between %d and %d: %d", MinTxGasLimit, MaxTxGasLimit, gasLimit)
	}

	return nil
}

func (*MsgInitiateWithdrawal) Type() string {
	return "l2_eth_withdrawal"
}

func (*MsgInitiateWithdrawal) Route() string {
	return ModuleName
}

var _ sdktypes.Msg = (*MsgInitiateERC20Withdrawal)(nil)

func (m *MsgInitiateERC20Withdrawal) ValidateBasic() error {
	// Check if the target Ethereum address is valid
	if !common.IsHexAddress(m.Target) {
		return fmt.Errorf("invalid target address: %s", m.Target)
	}
	// Check if the token address is valid
	if !common.IsHexAddress(m.TokenAddress) {
		return fmt.Errorf("invalid token address: %s", m.TokenAddress)
	}
	// Check if the gas limit is within the allowed range.
	gasLimit := new(big.Int).SetBytes(m.GasLimit).Uint64()
	if gasLimit < MinTxGasLimit || gasLimit > MaxTxGasLimit {
		return fmt.Errorf("gas limit must be between %d and %d: %d", MinTxGasLimit, MaxTxGasLimit, gasLimit)
	}

	return nil
}

func (*MsgInitiateERC20Withdrawal) Type() string {
	return "l2_erc20_withdrawal"
}

func (*MsgInitiateERC20Withdrawal) Route() string {
	return ModuleName
}

// TODO: Add validations for MsgUpdateParams and MsgInitiateFeeWithdrawal
