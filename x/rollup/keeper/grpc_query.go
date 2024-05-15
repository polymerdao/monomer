package keeper

import (
	"context"

	"github.com/polymerdao/monomer/gen/rollup/v1"
)

var _ rollupv1.QueryServiceServer = (*Keeper)(nil)

// L1BlockInfo implements the Query/L1BlockInfo gRPC method
// It returns the L1 block info. L2 clients should directly get L1 block info from x/rolllup keeper
func (k *Keeper) L1BlockInfo(ctx context.Context, request *rollupv1.L1BlockInfoRequest) (*rollupv1.L1BlockInfoResponse, error) {
	info, err := k.GetL1BlockInfo(ctx)
	if err != nil {
		return nil, err
	}
	return &rollupv1.L1BlockInfoResponse{
		Number:         info.Number,
		Time:           info.Time,
		BaseFee:        info.BaseFee.Bytes(),
		BlockHash:      info.BlockHash.Bytes(),
		SequenceNumber: info.SequenceNumber,
		BatcherAddr:    info.BatcherAddr.Bytes(),
		L1FeeOverhead:  info.L1FeeOverhead[:],
		L1FeeScalar:    info.L1FeeScalar[:],
	}, nil
}
