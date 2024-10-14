package tx

import (
	"errors"
	"fmt"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/polymerdao/monomer/x/rollup/tx/internal"
	"github.com/polymerdao/monomer/x/rollup/types"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type Deposit struct {
	Msg *types.MsgApplyL1Txs
}

var _ sdktypes.Tx = (*Deposit)(nil)

func (d *Deposit) GetMsgs() []proto.Message {
	return []proto.Message{d.Msg}
}

func (d *Deposit) GetMsgsV2() ([]protoreflect.ProtoMessage, error) {
	return []protoreflect.ProtoMessage{protov1.MessageV2(d.Msg)}, nil
}

// TODO split deposits into three types:
// 1. L1Info
// 2. Mint
// 3. ForceInclude
// also add an sdktypes.PreBlocker to check that these are in the propoer order.

func DepositAnteHandler(ctx sdktypes.Context, tx sdktypes.Tx, simulate bool) (sdktypes.Context, error) { //nolint:gocritic // hugeparam
	if _, ok := tx.(*Deposit); ok {
		ctx = ctx.WithGasMeter(internal.NewFreeInfiniteGasMeter())
	}
	return ctx, nil
}

// DepositDecoder is an sdktypes.TxDecoder.
func DepositDecoder(txBytes []byte) (sdktypes.Tx, error) {
	depositMsg := new(types.MsgApplyL1Txs)
	if err := proto.Unmarshal(txBytes, depositMsg); err != nil {
		return nil, fmt.Errorf("unmarshal proto: %v", err)
	}
	// Right now, MsgApplyL1Txs is not descriptive enough. Sometimes, non-deposit txs properly unmarshal.
	// For the time being, we do a few sanity checks here.
	if len(depositMsg.TxBytes) == 0 {
		return nil, errors.New("there must be at least one deposit tx")
	}
	var tx ethtypes.Transaction
	if err := tx.UnmarshalBinary(depositMsg.TxBytes[0]); err != nil {
		return nil, fmt.Errorf("cannot unmarshal l1 attributes tx: %v", err)
	}
	return &Deposit{
		Msg: depositMsg,
	}, nil
}
