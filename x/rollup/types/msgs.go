package types

import (
	"fmt"
	"math/big"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

const (
	MinTxGasLimit = params.TxGas
	MaxTxGasLimit = params.MaxGasLimit
)

var _ sdktypes.Msg = (*MsgApplyL1Txs)(nil)

func (m *MsgApplyL1Txs) ValidateBasic() error {
	if m.Txs == nil || len(m.Txs) < 1 {
		return WrapError(ErrInvalidL1Txs, "must have at least one L1 Info Deposit tx")
	}
	return nil
}

func (*MsgApplyL1Txs) Type() string {
	return "l1txs"
}

func (*MsgApplyL1Txs) Route() string {
	return "rollup"
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
	return "l2withdrawal"
}

func (*MsgInitiateWithdrawal) Route() string {
	return "rollup"
}
