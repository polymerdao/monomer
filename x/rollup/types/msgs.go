package types

import (
	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

// bank message types
const (
	TypeMsgL1Txs = "l1txs"
)

var (
	_ sdktypes.Msg = &MsgL1Txs{}
)

// NewMsgL1Txs creates a new MsgL1Txs instance.
func NewMsgL1Txs(txBytes [][]byte) *MsgL1Txs {
	return &MsgL1Txs{
		TxBytes: txBytes,
	}
}

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

func (msg *MsgL1Txs) Type() string {
	return TypeMsgL1Txs
}

func (msg *MsgL1Txs) Route() string {
	return RouterKey
}
