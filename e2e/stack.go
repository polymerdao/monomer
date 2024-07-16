package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"

	cometdb "github.com/cometbft/cometbft-db"
	"github.com/cometbft/cometbft/config"
	dbm "github.com/cosmos/cosmos-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
}

type Stack struct {
	monomerEngineURL *url.URL
	monomerCometURL  *url.URL
	opNodeURL        *url.URL
	deployConfigDir  string
	l1stateDumpDir   string
	eventListener    EventListener
	l1BlockTime      uint64
	prometheusCfg    *config.InstrumentationConfig
}

// New assumes all ports are available and that all paths exist and are valid.
func New(
	anvilURL,
	monomerEngineURL,
	monomerCometURL,
	opNodeURL *url.URL,
	deployConfigDir string,
	l1stateDumpDir string,
	l1BlockTime uint64,
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
		l1BlockTime:      l1BlockTime,
		prometheusCfg:    prometheusCfg,
	}
}

// helper shim to allow decoding of hex nonces
type auxDumpAccount struct {
	Balance     string                 `json:"balance"`
	Nonce       string                 `json:"nonce"`
	Root        hexutil.Bytes          `json:"root"`
	CodeHash    hexutil.Bytes          `json:"codeHash"`
	Code        hexutil.Bytes          `json:"code,omitempty"`
	Storage     map[common.Hash]string `json:"storage,omitempty"`
	Address     *common.Address        `json:"address,omitempty"` // Address only present in iterative (line-by-line) mode
	AddressHash hexutil.Bytes          `json:"key,omitempty"`     // If we don't have address, we can output the key
}

// helper shim to allow decoding of hex nonces
type auxDump struct {
	Root     string                    `json:"root"`
	Accounts map[string]auxDumpAccount `json:"accounts"`
	// Next can be set to represent that this dump is only partial, and Next
	// is where an iterator should be positioned in order to continue the dump.
	Next []byte `json:"next,omitempty"` // nil if no more accounts
}

func (d *auxDump) ToStateDump() (*state.Dump, error) {
	accounts := make(map[string]state.DumpAccount)

	for k, v := range d.Accounts { //nolint:gocritic
		nonce, err := hexutil.DecodeUint64(v.Nonce)
		if err != nil {
			return nil, fmt.Errorf("decode nonce: %v", err)
		}

		accounts[k] = state.DumpAccount{
			Balance:  v.Balance,
			Nonce:    nonce,
			Root:     v.Root,
			CodeHash: v.CodeHash,
			Code:     v.Code,
			Storage:  v.Storage,
		}
	}

	return &state.Dump{
		Root:     d.Root,
		Accounts: accounts,
		Next:     d.Next,
	}, nil
}

func (s *Stack) Run(ctx context.Context, env *environment.Env) error {
	// configure & run L1

	const networkName = "devnetL1"
	l1Deployments, err := opgenesis.NewL1Deployments(filepath.Join(s.l1stateDumpDir, "addresses.json"))
	if err != nil {
		return fmt.Errorf("new l1 deployments: %v", err)
	}
	deployConfig, err := opgenesis.NewDeployConfigWithNetwork(networkName, s.deployConfigDir)
	if err != nil {
		return fmt.Errorf("new deploy config: %v", err)
	}
	deployConfig.SetDeployments(l1Deployments)
	deployConfig.L1UseClique = false // Allows node to produce blocks without addition config. Clique is a PoA config.

	// Generate a deployer key and pre-fund the account
	deployerKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}

	var auxState auxDump

	l1StateJSON, err := os.ReadFile(filepath.Join(s.l1stateDumpDir, "allocs-l1.json"))
	if err != nil {
		// check if not found
		if os.IsNotExist(err) {
			return fmt.Errorf("allocs-l1.json not found - run `make setup-e2e` from project root")
		} else {
			return fmt.Errorf("read allocs-l1.json: %v", err)
		}
	}
	err = json.Unmarshal(l1StateJSON, &auxState)
	if err != nil {
		return fmt.Errorf("unmarshal l1 state: %v", err)
	}

	l1state, err := auxState.ToStateDump()
	if err != nil {
		return fmt.Errorf("auxState to state dump: %v", err)
	}

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, l1state, l1Deployments)
	if err != nil {
		return fmt.Errorf("build l1 developer genesis: %v", err)
	}

	l1client, l1HTTPendpoint, err := gethdevnet(s.l1BlockTime, l1genesis)
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
	if err := s.runMonomer(ctx, env, latestL1Block.Time(), deployConfig.L2ChainID); err != nil {
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
		s.prometheusCfg,
		s.eventListener,
	)
	if err := n.Run(ctx, env); err != nil {
		return fmt.Errorf("run monomer: %v", err)
	}
	return nil
}
