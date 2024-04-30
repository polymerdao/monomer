package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"os/exec"
	"path/filepath"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testutil/testapp"
	"github.com/polymerdao/monomer/utils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/sourcegraph/conc"
)

const oneETH = uint64(1e18)

type EventListener interface {
	OPEventListener

	OnCmdStart(programName string, stdout, stderr io.Reader)
	OnCmdStopped(programName string, err error)
}

type Stack struct {
	anvilURL         *url.URL
	monomerEngineURL *url.URL
	monomerCometURL  *url.URL
	opNodeURL        *url.URL
	contractsRootDir string
	eventListener    EventListener
	l1BlockTime      time.Duration
}

// New assumes all ports are available and that all paths exist and are valid.
func New(
	anvilURL,
	monomerEngineURL,
	monomerCometURL,
	opNodeURL *url.URL,
	contractsRootDir string,
	l1BlockTime time.Duration,
	eventListener EventListener,
) *Stack {
	return &Stack{
		anvilURL:         anvilURL,
		monomerEngineURL: monomerEngineURL,
		monomerCometURL:  monomerCometURL,
		opNodeURL:        opNodeURL,
		contractsRootDir: contractsRootDir,
		eventListener:    eventListener,
		l1BlockTime:      l1BlockTime,
	}
}

func (s *Stack) Run(parentCtx context.Context) (err error) {
	var wg conc.WaitGroup
	defer wg.Wait()
	ctx, cancel := context.WithCancelCause(parentCtx)
	defer func() {
		cancel(err)
		err = utils.Cause(ctx)
	}()

	// Run anvil.
	wg.Go(func() {
		cancel(s.runCmd(
			ctx,
			"anvil",
			"--port", s.anvilURL.Port(),
			"--order", "fifo",
			"--disable-block-gas-limit",
			"--gas-price", "0",
			"--block-time", fmt.Sprint(s.l1BlockTime.Seconds()),
		))
	})
	// NOTE: should we set a timeout on the context? Might not be worth the complexity.
	if !s.anvilURL.IsReachable(ctx) {
		return nil
	}

	// Fund an account.
	anvilRPCClient, err := rpc.DialContext(ctx, s.anvilURL.String())
	if err != nil {
		return fmt.Errorf("dial anvil: %v", err)
	}
	anvil := NewAnvilClient(anvilRPCClient)
	privKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}
	if err := anvil.SetBalance(ctx, crypto.PubkeyToAddress(privKey.PublicKey), 10*oneETH); err != nil { //nolint:gomnd
		return fmt.Errorf("set balance: %v", err)
	}

	// Deploy the OP L1 contracts.
	if err := s.runCmd(
		ctx,
		"forge",
		"script",
		"--root", s.contractsRootDir,
		"-vvv",
		fmt.Sprintf("%s:Deploy", filepath.Join(s.contractsRootDir, "scripts", "Deploy.s.sol")),
		"--rpc-url", s.anvilURL.String(),
		"--broadcast",
		"--private-key", common.Bytes2Hex(crypto.FromECDSA(privKey)),
	); err != nil {
		return fmt.Errorf("deploy op l1 contracts: %v", err)
	}
	latestL1Block, err := anvil.BlockByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("get the latest l1 block: %v", err)
	}

	// Run Monomer.
	const l2ChainID = 901
	wg.Go(func() {
		cancel(s.runMonomer(ctx, latestL1Block.Time(), l2ChainID))
	})
	if !s.monomerEngineURL.IsReachable(ctx) {
		return nil
	}
	monomerRPCClient, err := rpc.DialContext(ctx, s.monomerEngineURL.String())
	if err != nil {
		return fmt.Errorf("dial monomer: %v", err)
	}
	monomerClient := NewMonomerClient(monomerRPCClient)
	l2GenesisBlockHash, err := monomerClient.GenesisHash(ctx)
	if err != nil {
		return fmt.Errorf("get Monomer genesis block hash: %v", err)
	}

	// Get deploy config and rollup config.
	// The Optimism repo only includes configs for Hardhat. Fortunately, Anvil is designed to be compatible and works fine here.
	const networkName = "hardhat"
	l1Deployments, err := opgenesis.NewL1Deployments(filepath.Join(s.contractsRootDir, "deployments", networkName, ".deploy"))
	if err != nil {
		return fmt.Errorf("new l1 deployments: %v", err)
	}
	deployConfig, err := opgenesis.NewDeployConfigWithNetwork(networkName, filepath.Join(s.contractsRootDir, "deploy-config"))
	if err != nil {
		return fmt.Errorf("new deploy config: %v", err)
	}
	deployConfig.L1ChainID = 31337     // The file in the Optimism repo mistakenly sets the Hardhat L1 chain ID to 900.
	deployConfig.L2ChainID = l2ChainID // Ensure Monomer and the deploy config are aligned.
	deployConfig.SetDeployments(l1Deployments)
	rollupConfig, err := deployConfig.RollupConfig(latestL1Block, l2GenesisBlockHash, 1)
	if err != nil {
		return fmt.Errorf("new rollup config: %v", err)
	}

	opStack := NewOPStack(
		s.anvilURL,
		s.monomerEngineURL,
		s.opNodeURL,
		l1Deployments.L2OutputOracleProxy,
		privKey,
		rollupConfig,
		s.eventListener,
	)
	wg.Go(func() {
		err := opStack.Run(ctx)
		if err != nil {
			err = fmt.Errorf("run the op stack: %v", err)
		}
		cancel(err)
	})

	<-ctx.Done()
	return nil
}

func (s *Stack) runCmd(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe on %s: %v", name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe on %s: %v", name, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %v", name, err)
	}
	defer func() {
		s.eventListener.OnCmdStopped(name, cmd.Wait())
	}()
	s.eventListener.OnCmdStart(name, stdout, stderr)
	return nil
}

func (s *Stack) runMonomer(ctx context.Context, genesisTime, chainIDU64 uint64) error {
	engineHTTP, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("set up monomer engine http listener: %v", err)
	}
	engineWS, err := net.Listen("tcp", s.monomerEngineURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer engine ws listener: %v", err)
	}
	cometListener, err := net.Listen("tcp", s.monomerCometURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer comet listener: %v", err)
	}
	chainID := monomer.ChainID(chainIDU64)
	app := testapp.New(tmdb.NewMemDB(), chainID.String())
	n := node.New(app, &genesis.Genesis{
		AppState: app.DefaultGenesis(),
		ChainID:  chainID,
		Time:     genesisTime,
	}, engineHTTP, engineWS, cometListener, rolluptypes.AdaptCosmosTxsToEthTxs, rolluptypes.AdaptPayloadTxsToCosmosTxs)
	if err := n.Run(ctx); err != nil {
		return fmt.Errorf("run monomer: %v", err)
	}
	return nil
}
