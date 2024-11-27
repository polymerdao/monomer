package types

import (
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"google.golang.org/protobuf/reflect/protoreflect"
)

var _ sdktypes.Tx = (*DepositsTx)(nil)

func (d *DepositsTx) GetMsgs() []proto.Message {
	protoMsgUserDeposits := make([]proto.Message, 0, len(d.UserDeposits))
	for _, userDeposit := range d.UserDeposits {
		protoMsgUserDeposits = append(protoMsgUserDeposits, userDeposit)
	}
	return append([]proto.Message{d.L1Attributes}, protoMsgUserDeposits...)
}

func (d *DepositsTx) GetMsgsV2() ([]protoreflect.ProtoMessage, error) {
	msgsV1 := d.GetMsgs()
	msgs := make([]protoreflect.ProtoMessage, 0, len(msgsV1))
	for _, msgV1 := range msgsV1 {
		msgs = append(msgs, protov1.MessageV2(msgV1))
	}
	return msgs, nil
}
