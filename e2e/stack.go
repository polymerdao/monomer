package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
)

type EventListener interface {
	OPEventListener
	node.EventListener

	// HandleCmdOutput must not block. It is called at most once for a command.
	HandleCmdOutput(path string, stdout, stderr io.Reader)
	// err will never be nil.
	OnAnvilErr(err error)
}

type Stack struct {
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
		monomerEngineURL: monomerEngineURL,
		monomerCometURL:  monomerCometURL,
		opNodeURL:        opNodeURL,
		contractsRootDir: contractsRootDir,
		eventListener:    eventListener,
		l1BlockTime:      l1BlockTime,
	}
}

func (s *Stack) Run(ctx context.Context, env *environment.Env) error {
	// configure & run L1

	const l2ChainID = 901
	const networkName = "devnetL1"
	l1Deployments, err := opgenesis.NewL1Deployments(filepath.Join("optimism", ".devnet", "addresses.json"))
	if err != nil {
		return fmt.Errorf("new l1 deployments: %v", err)
	}
	deployConfig, err := opgenesis.NewDeployConfigWithNetwork(networkName, filepath.Join(s.contractsRootDir, "deploy-config"))
	if err != nil {
		return fmt.Errorf("new deploy config: %v", err)
	}
	deployConfig.L2ChainID = l2ChainID // Ensure Monomer and the deploy config are aligned.

	deployConfig.SetDeployments(l1Deployments)

	// Generate a deployer key and pre-fund the account
	deployerKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}

	var l1state state.Dump

	l1StateJSON, err := os.ReadFile(filepath.Join("optimism", ".devnet", "allocs-l1.json"))
	if err != nil {
		// check if not found
		if os.IsNotExist(err) {
			return fmt.Errorf("allocs-l1.json not found - run `make setup-e2e` from project root")
		} else {
			return fmt.Errorf("read allocs-l1.json: %v", err)
		}
	}
	err = json.Unmarshal(l1StateJSON, &l1state)
	if err != nil {
		return fmt.Errorf("unmarshal l1 state: %v", err)
	}

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, &l1state, l1Deployments)
	if err != nil {
		return fmt.Errorf("build l1 developer genesis: %v", err)
	}

	l1client, l1HTTPendpoint, err := ethdevnet(ctx, uint64(s.l1BlockTime.Seconds()), l1genesis)
	if err != nil {
		return fmt.Errorf("ethdevnet: %v", err)
	}

	l1url, err := url.ParseString(l1HTTPendpoint)
	if err != nil {
		return fmt.Errorf("new l1 url: %v", err)
	}

	// NOTE: should we set a timeout on the context? Might not be worth the complexity.
	if !l1url.IsReachable(ctx) {
		return fmt.Errorf("l1 url not reachable: %s", l1url.String())
	}

	l1 := NewL1Client(l1client)

	latestL1Block, err := l1.BlockByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("get the latest l1 block: %v", err)
	}

	// Run Monomer.
	if err := s.runMonomer(ctx, env, latestL1Block.Time(), l2ChainID); err != nil {
		return err
	}
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

	rollupConfig, err := deployConfig.RollupConfig(latestL1Block, l2GenesisBlockHash, 1)
	if err != nil {
		return fmt.Errorf("new rollup config: %v", err)
	}

	opStack := NewOPStack(
		l1url,
		s.monomerEngineURL,
		s.opNodeURL,
		l1Deployments.L2OutputOracleProxy,
		deployerKey,
		rollupConfig,
		s.eventListener,
	)
	if err := opStack.Run(ctx, env); err != nil {
		return fmt.Errorf("run the op stack: %v", err)
	}
	return nil
}

func (s *Stack) runMonomer(ctx context.Context, env *environment.Env, genesisTime, chainIDU64 uint64) error {
	engineWS, err := net.Listen("tcp", s.monomerEngineURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer engine ws listener: %v", err)
	}
	cometListener, err := net.Listen("tcp", s.monomerCometURL.Host())
	if err != nil {
		return fmt.Errorf("set up monomer comet listener: %v", err)
	}
	chainID := monomer.ChainID(chainIDU64)
	app, err := testapp.New(dbm.NewMemDB(), chainID.String())
	if err != nil {
		return fmt.Errorf("new test app: %v", err)
	}
	blockdb := dbm.NewMemDB()
	env.DeferErr("close block db", blockdb.Close)
	txdb := cometdb.NewMemDB()
	env.DeferErr("close tx db", txdb.Close)
	mempooldb := dbm.NewMemDB()
	env.DeferErr("close mempool db", mempooldb.Close)
	n := node.New(
		app,
		&genesis.Genesis{
			AppState: app.DefaultGenesis(),
			ChainID:  chainID,
			Time:     genesisTime,
		},
		engineWS,
		cometListener,
		blockdb,
		mempooldb,
		txdb,
		s.eventListener,
	)
	if err := n.Run(ctx, env); err != nil {
		return fmt.Errorf("run monomer: %v", err)
	}
	return nil
}
