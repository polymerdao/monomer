package e2e_test

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bftclient "github.com/cometbft/cometbft/rpc/client/http"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-bindings/bindings"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer/e2e"
	e2eurl "github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slog"
)

const artifactsDirectoryName = "artifacts"

func openLogFile(t *testing.T, env *environment.Env, name string) *os.File {
	filename := filepath.Join(artifactsDirectoryName, name+".log")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	env.DeferErr("close log file: "+filename, file.Close)
	return file
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	const l1BlockTime = uint64(1) // We would want 250ms instead, but blocktimes are uints in seconds.

	deployConfigDir, err := filepath.Abs("./optimism/packages/contracts-bedrock/deploy-config")
	require.NoError(t, err)
	l1StateDumpDir, err := filepath.Abs("./optimism/.devnet")
	require.NoError(t, err)

	monomerEngineURL := newURL(t, "ws://127.0.0.1:8889")
	monomerCometURL := newURL(t, "http://127.0.0.1:8890")
	opNodeURL := newURL(t, "http://127.0.0.1:8891")

	env := environment.New()
	defer func() {
		require.NoError(t, env.Close())
	}()

	if err := os.Mkdir(artifactsDirectoryName, 0o755); !errors.Is(err, os.ErrExist) {
		require.NoError(t, err)
	}
	opLogger := log.NewTerminalHandler(openLogFile(t, env, "op"), false)

	stack := e2e.New(monomerEngineURL, monomerCometURL, opNodeURL, deployConfigDir, l1StateDumpDir, l1BlockTime, &e2e.SelectiveListener{
		OPLogCb: func(r slog.Record) {
			require.NoError(t, opLogger.Handle(context.Background(), r))
		},
		NodeSelectiveListener: &node.SelectiveListener{
			OnEngineHTTPServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnEngineWebsocketServeErrCb: func(err error) {
				require.NoError(t, err)
			},
			OnCometServeErrCb: func(err error) {
				require.NoError(t, err)
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ruConfig, err := stack.Run(ctx, env)
	require.NoError(t, err)
	// To avoid flaky tests, hang until the Monomer server is ready.
	// We rely on the `go test` timeout to ensure the tests don't hang forever (default is 10 minutes).
	require.True(t, monomerEngineURL.IsReachable(ctx))
	monomerRPCClient, err := rpc.DialContext(ctx, monomerEngineURL.String())
	require.NoError(t, err)
	monomerClient := e2e.NewMonomerClient(monomerRPCClient)

	const targetHeight = 5

	// construct L1 client. Hang until L1 responsive.
	fmt.Println("Waiting for L1 to be reachable")
	fmt.Println("L1 URL: ", ruConfig.L1URL.String())
	require.True(t, ruConfig.L1URL.IsReachable(ctx))
	fmt.Println("L1 is reachable")
	l1RPCClient, err := rpc.DialContext(ctx, ruConfig.L1URL.String())
	require.NoError(t, err)
	l1Client := e2e.NewL1Client(l1RPCClient)

	// instantiate L1 user, tx signer.
	user := stack.L1Users[0]
	signer := types.NewEIP155Signer(ruConfig.L1ChainID)

	// op Portal
	portal, err := bindings.NewOptimismPortal(ruConfig.DepositContractAddress, l1Client)
	require.NoError(t, err)

	// send user Deposit Tx
	nonce, err := l1Client.Client.NonceAt(ctx, user.Address, nil)
	require.NoError(t, err)

	price, err := l1Client.Client.SuggestGasPrice(context.Background())
	require.NoError(t, err)

	gasPrice := new(big.Int).Mul(price, big.NewInt(2))
	if gasPrice.BitLen() > 256 {
		gasPrice = price // fallback to original price if overflow
	}

	depositTx, err := portal.DepositTransaction(
		&bind.TransactOpts{
			From: user.Address,
			Signer: func(addr common.Address, tx *types.Transaction) (*types.Transaction, error) {
				signed, err := types.SignTx(tx, signer, user.PrivateKey)
				if err != nil {
					return nil, err
				}
				return signed, nil
			},
			Nonce:    big.NewInt(int64(nonce)),
			GasPrice: big.NewInt(price.Int64() * 2),
			GasLimit: 1e6,
			Value:    big.NewInt(1e18 / 10), // 0.1 eth
			Context:  ctx,
			NoSend:   false,
		},
		user.Address,
		big.NewInt(1e18/20), // 0.5 eth deposit to L2
		ruConfig.Genesis.SystemConfig.GasLimit/10, // 10% of block gas limit
		false,    // _isCreation
		[]byte{}, // no data
	)
	require.NoError(t, err, "deposit tx")

	bftClient, err := bftclient.New(monomerCometURL.String(), monomerCometURL.String())
	require.NoError(t, err, "create Comet client")

	txBytes := testapp.ToTx(t, "userTxKey", "userTxValue")
	bftTx := bfttypes.Tx(txBytes)

	putTx, err := bftClient.BroadcastTxAsync(ctx, txBytes)
	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, putTx.Code, "put.Code is not OK")
	require.EqualValues(t, bftTx.Hash(), putTx.Hash, "put.Hash does not match local hash")
	t.Log("Monomer can ingest cometbft txs")

	badPutTx := []byte("malformed")
	badPut, err := bftClient.BroadcastTxAsync(ctx, badPutTx)
	require.NoError(t, err) // no API error - failure encoded in response
	require.NotEqual(t, badPut.Code, abcitypes.CodeTypeOK, "badPut.Code is OK")
	t.Log("Monomer can reject malformed cometbft txs")

	checkTicker := time.NewTicker(time.Duration(l1BlockTime) * time.Second)
	defer checkTicker.Stop()
	for range checkTicker.C {
		latestBlock, err := monomerClient.BlockByNumber(context.Background(), nil)
		require.NoError(t, err)
		if latestBlock.NumberU64() >= targetHeight {
			break
		}
	}
	t.Log("Monomer can sync")

	getTx, err := bftClient.Tx(ctx, bftTx.Hash(), false)

	require.NoError(t, err)
	require.Equal(t, abcitypes.CodeTypeOK, getTx.TxResult.Code, "txResult.Code is not OK")
	require.Equal(t, bftTx, getTx.Tx, "txBytes do not match")
	t.Log("Monomer can serve txs by hash")

	txBlock, err := monomerClient.BlockByNumber(ctx, big.NewInt(getTx.Height))
	require.NoError(t, err)
	require.Len(t, txBlock.Transactions(), 2)

	// inspect L1 for deposit tx receipt
	receipt, err := l1Client.Client.TransactionReceipt(ctx, depositTx.Hash())
	require.NoError(t, err, "deposit tx receipt")
	require.NotNil(t, receipt, "deposit tx receipt")

	for i := uint64(2); i < targetHeight; i++ {
		block, err := monomerClient.BlockByNumber(ctx, new(big.Int).SetUint64(i))
		require.NoError(t, err)
		txs := block.Transactions()
		require.GreaterOrEqual(t, len(txs), 1, "expected at least 1 tx in block")
		if tx := txs[0]; !tx.IsDepositTx() {
			txBytes, err := tx.MarshalJSON()
			require.NoError(t, err)
			require.Fail(t, fmt.Sprintf("expected tx to be deposit tx: %s", txBytes))
		}
	}
	t.Log("Monomer blocks contain the l1 attributes deposit tx")
}

func newURL(t *testing.T, address string) *e2eurl.URL {
	stdURL, err := url.Parse(address)
	require.NoError(t, err)
	resultURL, err := e2eurl.Parse(stdURL)
	require.NoError(t, err)
	return resultURL
}
