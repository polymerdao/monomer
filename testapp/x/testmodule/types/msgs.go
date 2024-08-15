package types

import sdktypes "github.com/cosmos/cosmos-sdk/types"

var _ sdktypes.Msg = (*MsgSetValue)(nil)

func (m *MsgSetValue) ValidateBasic() error {
	return nil
}
