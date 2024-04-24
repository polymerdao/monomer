package testappv1

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func (*SetRequest) ValidateBasic() error {
	return nil
}

func (*SetRequest) GetSigners() []sdk.AccAddress {
	return nil
}

func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	msgservice.RegisterMsgServiceDesc(registry, &_SetService_serviceDesc)
}
