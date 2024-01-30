package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
)

var _ types.QueryServer = Keeper{}

// L1BlockInfo implements the Query/L1BlockInfo gRPC method
// It returns the L1 block info. L2 clients should directly get L1 block info from x/rolllup keeper
func (k Keeper) L1BlockInfo(goCtx context.Context, request *types.QueryL1BlockInfoRequest) (*types.QueryL1BlockInfoResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	l1blockInfo, err := k.GetL1BlockInfo(k.rollupCfg, ctx)
	if err != nil {
		return nil, err
	}
	return types.ToL1BlockInfoResponse(l1blockInfo), nil
}
