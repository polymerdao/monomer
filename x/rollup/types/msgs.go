package types

import (
	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

var _ sdktypes.Msg = &MsgL1Txs{}

// GetSigners implements types.Msg.
func (*MsgL1Txs) GetSigners() []sdktypes.AccAddress {
	return nil
}

// ValidateBasic implements types.Msg.
func (m *MsgL1Txs) ValidateBasic() error {
	if m.TxBytes == nil || len(m.TxBytes) < 1 {
		return WrapError(ErrInvalidL1Txs, "must have L1 system Deposit tx")
	}
	return nil
}

func (*MsgL1Txs) Type() string {
	return "l1txs"
}

func (*MsgL1Txs) Route() string {
	return RouterKey
}
