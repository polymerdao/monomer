package keeper_test

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
)

func (s *KeeperTestSuite) TestParamsQuery() {
	params := types.DefaultParams()
	err := s.rollupKeeper.SetParams(sdk.UnwrapSDKContext(s.ctx), &params)
	s.Require().NoError(err)

	response, err := s.rollupKeeper.Params(s.ctx, &types.QueryParamsRequest{})
	s.Require().NoError(err)
	s.Require().Equal(&types.QueryParamsResponse{Params: params}, response)
}

func (s *KeeperTestSuite) TestL1BlockInfoQuery() {
	l1BlockInfo := types.L1BlockInfo{
		Number: 1,
		Time:   1,
	}

	err := s.rollupKeeper.SetL1BlockInfo(sdk.UnwrapSDKContext(s.ctx), l1BlockInfo)
	s.Require().NoError(err)

	response, err := s.rollupKeeper.L1BlockInfo(s.ctx, &types.QueryL1BlockInfoRequest{})
	s.Require().NoError(err)
	s.Require().Equal(&types.QueryL1BlockInfoResponse{L1BlockInfo: l1BlockInfo}, response)
}
