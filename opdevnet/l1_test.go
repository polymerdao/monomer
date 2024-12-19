package opdevnet_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/opdevnet"
	"github.com/stretchr/testify/require"
)

func makeL1Config(t *testing.T) *opdevnet.L1Config {
	l1Deployments, err := opdevnet.DefaultL1Deployments()
	require.NoError(t, err)
	deployConfig, err := opdevnet.DefaultDeployConfig(l1Deployments)
	require.NoError(t, err)
	l1URL, err := e2eurl.ParseString("ws://127.0.0.1:8892")
	require.NoError(t, err)
	beaconURL, err := e2eurl.ParseString("ws://127.0.0.1:8893")
	require.NoError(t, err)
	l1Allocs, err := opdevnet.DefaultL1Allocs()
	require.NoError(t, err)

	l1Config, err := opdevnet.BuildL1Config(
		deployConfig,
		l1Deployments,
		l1Allocs,
		l1URL,
		beaconURL,
		t.TempDir(),
	)
	require.NoError(t, err)
	l1Config.BlobsDirPath = t.TempDir()

	return l1Config
}

func TestWebsocketAndHTTPWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := environment.New()
	defer func() {
		cancel()
		require.NoError(t, env.Close())
	}()

	l1Config := makeL1Config(t)
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

func TestL1ProducesBlocks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	env := environment.New()
	defer func() {
		cancel()
		require.NoError(t, env.Close())
	}()

	l1Config := makeL1Config(t)
	require.NoError(t, l1Config.Run(ctx, env, log.NewLogger(slog.NewTextHandler(testLog{t: t}, nil))))

	l1Client, err := ethclient.Dial(l1Config.URL.String())
	require.NoError(t, err)
	for {
		number, err := l1Client.BlockNumber(ctx)
		require.NoError(t, err)
		if number > 0 {
			break
		}
		time.Sleep(time.Duration(l1Config.BlockTime) * time.Second)
	}
}

type testLog struct {
	t *testing.T
}

func (tl testLog) Write(data []byte) (int, error) {
	tl.t.Log(string(data))
	return len(data), nil
}
