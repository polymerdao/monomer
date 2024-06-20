package integrations

import (
	"context"

	"github.com/cosmos/cosmos-sdk/server"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	"golang.org/x/sync/errgroup"
)

const (
	FlagTraceStore = "trace-store"
	FlagGRPCOnly   = "grpc-only"
)

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L403
func getAndValidateConfig(svrCtx *server.Context) (serverconfig.Config, error) {
	config, err := serverconfig.GetConfig(svrCtx.Viper)
	if err != nil {
		return config, err
	}

	if err := config.ValidateBasic(); err != nil {
		return config, err
	}
	return config, nil
}

// See https://github.com/cosmos/cosmos-sdk/blob/7fb26685cd68a6c1d199dc270c80f49f2bfe7ace/server/start.go#L616
func getCtx(svrCtx *server.Context, block bool) (*errgroup.Group, context.Context) {
	ctx, cancelFn := context.WithCancel(context.Background())
	g, ctx := errgroup.WithContext(ctx)
	server.ListenForQuitSignals(g, block, cancelFn, svrCtx.Logger)
	return g, ctx
}
