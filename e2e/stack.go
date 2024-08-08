package e2e

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/e2e/url"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/genesis"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/node"
	"github.com/polymerdao/monomer/testapp"
)

type EventListener interface {
	OPEventListener
	node.EventListener
}

type StackConfig struct {
	L1URL *url.URL
	*rollup.Config
	Operator L1User
	Users    []L1User
}

type Stack struct {
	monomerEngineURL *url.URL
	monomerCometURL  *url.URL
	opNodeURL        *url.URL
	deployConfigDir  string
	l1stateDumpDir   string
	eventListener    EventListener
	prometheusCfg    *config.InstrumentationConfig
}

type L1User struct {
	Address    common.Address
	PrivateKey *ecdsa.PrivateKey
}

// New assumes all ports are available and that all paths exist and are valid.
func New(
	anvilURL,
	monomerEngineURL,
	monomerCometURL,
	opNodeURL *url.URL,
	deployConfigDir string,
	l1stateDumpDir string,
	prometheusCfg *config.InstrumentationConfig,
	eventListener EventListener,
) *Stack {
	return &Stack{
		monomerEngineURL: monomerEngineURL,
		monomerCometURL:  monomerCometURL,
		opNodeURL:        opNodeURL,
		deployConfigDir:  deployConfigDir,
		l1stateDumpDir:   l1stateDumpDir,
		eventListener:    eventListener,
		prometheusCfg:    prometheusCfg,
	}
}

func (s *Stack) Run(ctx context.Context, env *environment.Env) (*StackConfig, error) {
	// configure & run L1

	const networkName = "devnetL1"
	l1Deployments, err := opgenesis.NewL1Deployments(filepath.Join(s.l1stateDumpDir, "addresses.json"))
	if err != nil {
		return nil, fmt.Errorf("new l1 deployments: %v", err)
	}
	deployConfig, err := opgenesis.NewDeployConfigWithNetwork(networkName, s.deployConfigDir)
	if err != nil {
		return nil, fmt.Errorf("new deploy config: %v", err)
	}
	deployConfig.SetDeployments(l1Deployments)
	deployConfig.L1GenesisBlockTimestamp = hexutil.Uint64(time.Now().Unix())
	deployConfig.L1UseClique = false // Allows node to produce blocks without addition config. Clique is a PoA config.
	// SequencerWindowSize is usually set to something in the hundreds or thousands.
	// That means we don't ever perform unsafe block consolidation (i.e., the safe head never advances) before the test is complete.
	// To force this edge case to occur in the test, we decrease the SWS.
	deployConfig.SequencerWindowSize = 4
	// Set low ChannelTimeout to ensure the batcher opens and closes a channel in the same block to avoid reorgs in
	// unsafe block consolidation. Note that at the time of this writing, we're still seeing reorgs, so this likely isn't the silver bullet.
	deployConfig.ChannelTimeout = 1
	deployConfig.L1BlockTime = 2
	deployConfig.L2BlockTime = 1

	var auxState auxDump

	l1StateJSON, err := os.ReadFile(filepath.Join(s.l1stateDumpDir, "allocs-l1.json"))
	if err != nil {
		// check if not found
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("allocs-l1.json not found - run `make setup-e2e` from project root")
		} else {
			return nil, fmt.Errorf("read allocs-l1.json: %v", err)
		}
	}
	err = json.Unmarshal(l1StateJSON, &auxState)
	if err != nil {
		return nil, fmt.Errorf("unmarshal l1 state: %v", err)
	}

	l1state, err := auxState.ToStateDump()
	if err != nil {
		return nil, fmt.Errorf("auxState to state dump: %v", err)
	}

	// Fund test EOA accounts
	l1users := make([]L1User, 0)
	for range 2 {
		privateKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key: %v", err)
		}
		userAddress := crypto.PubkeyToAddress(privateKey.PublicKey)

		user := state.DumpAccount{
			Balance: "0x100000000000000000000000000000000000000000000000000000000000000",
			Nonce:   0,
			Address: &userAddress,
		}
		l1state.OnAccount(user.Address, user)
		l1users = append(l1users, L1User{
			Address:    userAddress,
			PrivateKey: privateKey,
		})
	}

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, l1state, l1Deployments)
	if err != nil {
		return nil, fmt.Errorf("build l1 developer genesis: %v", err)
	}

	l1client, l1HTTPendpoint, err := gethdevnet(env, deployConfig.L1BlockTime, l1genesis)
	if err != nil {
		return nil, fmt.Errorf("ethdevnet: %v", err)
	}

	l1url, err := url.ParseString(l1HTTPendpoint)
	if err != nil {
		return nil, fmt.Errorf("new l1 url: %v", err)
	}

	// NOTE: should we set a timeout on the context? Might not be worth the complexity.
	if !l1url.IsReachable(ctx) {
		return nil, fmt.Errorf("l1 url not reachable: %s", l1url.String())
	}

	l1 := NewL1Client(l1client)

	latestL1Block, err := l1.BlockByNumber(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("get the latest l1 block: %v", err)
	}

	// Run Monomer.
	if err := s.runMonomer(ctx, env, latestL1Block.Time(), deployConfig.L2ChainID); err != nil {
		return nil, err
	}
	if !s.monomerEngineURL.IsReachable(ctx) {
		return nil, fmt.Errorf("reaching monomer url: %s", s.monomerEngineURL.String())
	}
	monomerRPCClient, err := rpc.DialContext(ctx, s.monomerEngineURL.String())
	if err != nil {
		return nil, fmt.Errorf("dial monomer: %v", err)
	}
	monomerClient := NewMonomerClient(monomerRPCClient)
	l2GenesisBlockHash, err := monomerClient.GenesisHash(ctx)
	if err != nil {
		return nil, fmt.Errorf("get Monomer genesis block hash: %v", err)
	}

	rollupConfig, err := deployConfig.RollupConfig(latestL1Block, l2GenesisBlockHash, 1)
	if err != nil {
		return nil, fmt.Errorf("new rollup config: %v", err)
	}

	opStack := NewOPStack(
		l1url,
		s.monomerEngineURL,
		s.opNodeURL,
		l1Deployments.L2OutputOracleProxy,
		l1users[0].PrivateKey,
		rollupConfig,
		s.eventListener,
	)
	if err := opStack.Run(ctx, env); err != nil {
		return nil, fmt.Errorf("run the op stack: %v", err)
	}

	return &StackConfig{
		L1URL:    l1url,
		Config:   rollupConfig,
		Operator: l1users[0],
		Users:    l1users[1:],
	}, nil
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
	blockPebbleDB, err := pebble.Open("", &pebble.Options{
		FS: vfs.NewMem(),
	})
	if err != nil {
		return fmt.Errorf("open block db: %v", err)
	}
	env.DeferErr("close block db", blockPebbleDB.Close)
	txdb := cometdb.NewMemDB()
	env.DeferErr("close tx db", txdb.Close)
	mempooldb := dbm.NewMemDB()
	env.DeferErr("close mempool db", mempooldb.Close)
	ethstatedb := state.NewDatabase(rawdb.NewMemoryDatabase())
	env.DeferErr("close eth state disk db", ethstatedb.DiskDB().Close)
	env.DeferErr("close eth state trie db", ethstatedb.TrieDB().Close)
	n := node.New(
		app,
		&genesis.Genesis{
			AppState: app.DefaultGenesis(),
			ChainID:  chainID,
			Time:     genesisTime,
		},
		engineWS,
		cometListener,
		localdb.New(blockPebbleDB),
		mempooldb,
		txdb,
		ethstatedb,
		s.prometheusCfg,
		s.eventListener,
	)
	if err := n.Run(ctx, env); err != nil {
		return fmt.Errorf("run monomer: %v", err)
	}
	return nil
}
