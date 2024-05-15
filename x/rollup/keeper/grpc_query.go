package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/gen/rollup/v1"
)

var _ rollup_v1.QueryServiceServer = (*Keeper)(nil)

// L1BlockInfo implements the Query/L1BlockInfo gRPC method
// It returns the L1 block info. L2 clients should directly get L1 block info from x/rolllup keeper
func (k *Keeper) L1BlockInfo(
	goCtx context.Context,
	request *rollup_v1.L1BlockInfoRequest,
) (*rollup_v1.L1BlockInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	l1blockInfo, err := k.GetL1BlockInfo(k.rollupCfg, &ctx)
	if err != nil {
		return nil, err
	}
	return &rollup_v1.L1BlockInfoResponse{
		Number:         l1blockInfo.Number,
		Time:           l1blockInfo.Time,
		BaseFee:        l1blockInfo.BaseFee.Bytes(),
		BlockHash:      l1blockInfo.BlockHash.Bytes(),
		SequenceNumber: l1blockInfo.SequenceNumber,
		BatcherAddr:    l1blockInfo.BatcherAddr.Bytes(),
		L1FeeOverhead:  l1blockInfo.L1FeeOverhead[:],
		L1FeeScalar:    l1blockInfo.L1FeeScalar[:],
	}, nil
}
