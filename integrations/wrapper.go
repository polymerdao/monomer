package integrations

import (
	"context"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
)

type WrappedApplication struct {
	app servertypes.Application
}

func (wa *WrappedApplication) RollbackToHeight(_ context.Context, targetHeight uint64) error {
	return wa.app.CommitMultiStore().RollbackToVersion(int64(targetHeight))
}

func (wa *WrappedApplication) Info(_ context.Context, req *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error) {
	return wa.app.Info(req)
}

func (wa *WrappedApplication) CheckTx(_ context.Context, req *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error) {
	return wa.app.CheckTx(req)
}

func (wa *WrappedApplication) Commit(_ context.Context, _ *abcitypes.RequestCommit) (*abcitypes.ResponseCommit, error) {
	return wa.app.Commit()
}

func (wa *WrappedApplication) FinalizeBlock(
	_ context.Context,
	req *abcitypes.RequestFinalizeBlock,
) (*abcitypes.ResponseFinalizeBlock, error) {
	return wa.app.FinalizeBlock(req)
}

func (wa *WrappedApplication) InitChain(_ context.Context, req *abcitypes.RequestInitChain) (*abcitypes.ResponseInitChain, error) {
	return wa.app.InitChain(req)
}

func (wa *WrappedApplication) Query(ctx context.Context, req *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error) {
	return wa.app.Query(ctx, req)
}
