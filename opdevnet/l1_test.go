package opdevnet_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/stretchr/testify/require"
)

func TestWebsocketAndHTTPWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := environment.New()
	defer func() {
		cancel()
		require.NoError(t, env.Close())
	}()

	l1Deployments, err := opdevnet.DefaultL1Deployments()
	require.NoError(t, err)
	deployConfig, err := opdevnet.DefaultDeployConfig(l1Deployments)
	require.NoError(t, err)
	l1URL, err := e2eurl.ParseString("ws://127.0.0.1:8892")
	require.NoError(t, err)
	l1Allocs, err := opdevnet.DefaultL1Allocs()
	require.NoError(t, err)

	l1Config, err := opdevnet.BuildL1Config(
		deployConfig,
		l1Deployments,
		l1Allocs,
		l1URL,
		t.TempDir(),
	)
	require.NoError(t, err)

	l1Config.BlobsDirPath = t.TempDir()
	require.NoError(t, l1Config.Run(ctx, env, log.NewLogger(log.DiscardHandler())))

	wsClient, err := ethclient.Dial(l1Config.URL.String())
	require.NoError(t, err)
	_, err = wsClient.ChainID(ctx)
	require.NoError(t, err)

	httpURL, err := e2eurl.ParseString("http://" + l1Config.URL.Host())
	require.NoError(t, err)
	httpClient, err := ethclient.Dial(httpURL.String())
	require.NoError(t, err)
	_, err = httpClient.ChainID(ctx)
	require.NoError(t, err)
}
