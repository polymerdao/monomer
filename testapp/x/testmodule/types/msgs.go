package types

import sdktypes "github.com/cosmos/cosmos-sdk/types"

var _ sdktypes.Msg = (*SetRequest)(nil)

func (m *SetRequest) ValidateBasic() error {
	return nil
}
