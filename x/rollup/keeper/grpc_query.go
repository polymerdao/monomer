package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Querier struct {
	Keeper
}

var _ types.QueryServer = &Keeper{}

func NewQuerier(keeper *Keeper) Querier {
	return Querier{Keeper: *keeper}
}

// L1BlockInfo implements the Query/L1BlockInfo gRPC method
func (k Keeper) L1BlockInfo( //nolint:gocritic // hugeParam
	ctx context.Context,
	req *types.QueryL1BlockInfoRequest,
) (*types.QueryL1BlockInfoResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	l1BlockInfo, err := k.GetL1BlockInfo(sdk.UnwrapSDKContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get L1 block info: %w", err)
	}
	return &types.QueryL1BlockInfoResponse{L1BlockInfo: *l1BlockInfo}, nil
}

// Params implements the Query/Params gRPC method
func (k Keeper) Params( //nolint:gocritic // hugeParam
	ctx context.Context,
	req *types.QueryParamsRequest,
) (*types.QueryParamsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "invalid request")
	}
	params, err := k.GetParams(sdk.UnwrapSDKContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("get params: %w", err)
	}
	return &types.QueryParamsResponse{Params: *params}, nil
}
