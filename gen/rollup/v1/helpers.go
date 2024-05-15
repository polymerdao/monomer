package rollup_v1

import (
	"errors"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

var _ sdktypes.Msg = (*ApplyL1TxsRequest)(nil)

func (*ApplyL1TxsRequest) GetSigners() []sdktypes.AccAddress {
	return nil
}

func (m *ApplyL1TxsRequest) ValidateBasic() error {
	if len(m.TxBytes) < 1 {
		return errors.New("expected TxBytes to contain at least one deposit transaction")
	}
	return nil
}

func (*ApplyL1TxsRequest) Type() string {
	return "l1txs"
}

func (*ApplyL1TxsRequest) Route() string {
	return "rollup"
}

func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &_MsgService_serviceDesc)
}
