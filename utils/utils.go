package utils

import (
	"fmt"
	"io"

	sdktypes "github.com/cosmos/cosmos-sdk/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum/go-ethereum/common"
	"github.com/hashicorp/go-multierror"
	"github.com/polymerdao/monomer/x/rollup/types"
)

func Ptr[T any](x T) *T {
	return &x
}

func WrapCloseErr(err error, closer io.Closer) error {
	closeErr := closer.Close()
	if closeErr != nil {
		closeErr = fmt.Errorf("close: %v", closeErr)
	}
	if err != nil || closeErr != nil {
		return multierror.Append(err, closeErr)
	}
	return nil
}

// EvmToCosmosAddress converts an EVM address to a sdktypes.AccAddress
func EvmToCosmosAddress(addr common.Address) sdktypes.AccAddress {
	return addr.Bytes()
}

// DeriveL1BlockInfoToProto converts a derive.L1BlockInfo to our L1BlockInfo proto message
func DeriveL1BlockInfoToProto(info *derive.L1BlockInfo) *types.L1BlockInfo {
	protoL1BlockInfo := &types.L1BlockInfo{
		Number:            info.Number,
		Time:              info.Time,
		BlockHash:         info.BlockHash[:],
		SequenceNumber:    info.SequenceNumber,
		BatcherAddr:       info.BatcherAddr[:],
		L1FeeOverhead:     info.L1FeeOverhead[:],
		L1FeeScalar:       info.L1FeeScalar[:],
		BaseFeeScalar:     info.BaseFeeScalar,
		BlobBaseFeeScalar: info.BlobBaseFeeScalar,
	}

	if info.BaseFee != nil {
		protoL1BlockInfo.BaseFee = info.BaseFee.Bytes()
	}
	if info.BlobBaseFee != nil {
		protoL1BlockInfo.BlobBaseFee = info.BlobBaseFee.Bytes()
	}

	return protoL1BlockInfo
}
