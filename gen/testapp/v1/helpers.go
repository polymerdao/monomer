package testappv1

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

var _ sdktypes.Msg = (*SetRequest)(nil)

func (m *SetRequest) ValidateBasic() error {
	return nil
}

func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &_SetService_serviceDesc)
}
