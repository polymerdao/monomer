package integrations

import (
	"context"
	"io"
	"testing"

	"cosmossdk.io/log"
	abcitypes "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/rpc"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	testapp "github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
)

func mockAppCreator(
	_ log.Logger,
	db dbm.DB,
	_ io.Writer,
	_ servertypes.AppOptions,
) servertypes.Application {
	app, err := testapp.New(db, "testapp")
	if err != nil {
		panic(err)
	}

	return &WrappedTestApp{App: app}
}

func TestStartCommandHandler(t *testing.T) {
	svrCtx := server.NewDefaultContext()
	svrCtx.Viper.Set("minimum-gas-prices", "0.025stake")

	clientCtx := client.Context{}
	inProcessConsensus := true
	opts := server.StartCmdOptions{}

	err := StartCommandHandler(svrCtx, clientCtx, mockAppCreator, inProcessConsensus, opts)
	require.NoError(t, err)

	_, err = rpc.GetChainHeight(clientCtx)
	require.NoError(t, err)
}

type WrappedTestApp struct {
	*testapp.App
}

func (w *WrappedTestApp) ListSnapshots(_ *abcitypes.RequestListSnapshots) (*abcitypes.ResponseListSnapshots, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) OfferSnapshot(_ *abcitypes.RequestOfferSnapshot) (*abcitypes.ResponseOfferSnapshot, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) LoadSnapshotChunk(_ *abcitypes.RequestLoadSnapshotChunk) (*abcitypes.ResponseLoadSnapshotChunk, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) ApplySnapshotChunk(_ *abcitypes.RequestApplySnapshotChunk) (*abcitypes.ResponseApplySnapshotChunk, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) PrepareProposal(_ *abcitypes.RequestPrepareProposal) (*abcitypes.ResponsePrepareProposal, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) ProcessProposal(_ *abcitypes.RequestProcessProposal) (*abcitypes.ResponseProcessProposal, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) ExtendVote(_ context.Context, _ *abcitypes.RequestExtendVote) (*abcitypes.ResponseExtendVote, error) {
	panic("not implemented")
}

func (w *WrappedTestApp) VerifyVoteExtension(_ *abcitypes.RequestVerifyVoteExtension) (*abcitypes.ResponseVerifyVoteExtension, error) {
	panic("not implemented")
}
