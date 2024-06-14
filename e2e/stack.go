package e2e

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"os/exec"
	"path/filepath"
	"time"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/common"
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

const oneETH = uint64(1e18)

type EventListener interface {
	OPEventListener
	node.EventListener

	// HandleCmdOutput must not block. It is called at most once for a command.
	HandleCmdOutput(path string, stdout, stderr io.Reader)
	// err will never be nil.
	OnAnvilErr(err error)
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

func (s *Stack) Run(ctx context.Context, env *environment.Env) error {
	// configure & run L1

	const l2ChainID = 901
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

	// Generate a deployer key and pre-fund the account
	deployerKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}
	deployerAddr := crypto.PubkeyToAddress(deployerKey.PublicKey)
	balance := big.NewInt(0).Mul(big.NewInt(int64(oneETH)), big.NewInt(10))

	dumpHex := "0x1f8b08000000000000ffad55db6a1c310cfd9779ce837c9125e5671649b6c3d26437ec6e4a4ac8bf57b39b160225b4743c307864e99c235963bf2df678f46fcbfddb727879b2715aee177885e56ef1e3fe607a1e37c35f8e88bbec9fc6f9a24fcfd7c01496073def1ef74ffbcbcde2e28557cf157d8e1b413151d72b40dfcfb9f797c7cb8fdf5a9e4fe3fb490f5d8fffa4e60b9591b5edc6ab8ff379b7ea0bf0ddf369ef63adc487fdea138bcb3d7c04c4c787537a7fbf5bd4fdf872b89cd79848ac76279156bb6607b65c9ab0955edcc9b5e424a58b205e6b7d3cac282bae3eea751e08398d24035c4c2dd75f52fdd86fcb313f5f8e277d5845aefcc189bd54d5893537ea469dbc34e22261a314f50c294ddba69c9157e2c1699489de2705b997de3066e6a036d10294e7969cc56bf5debb351580a9d990b1f72c01537aca536b9662be25671d28562b57b24282c4c82c195c89a7cddc1cac0ab62f383f31d0fcbf118936482d7473bc33702ec4509073ccea448684091b94556ae23c3b1a87176684f589e01ad13ecb1f7225102121704c9c72f7a290c0a9433284013dd1dadbce5bd6576092d86cc3b2d7c914f9e1882a0be791668234624705daa69cd4865285dc06e58658a3a390a662a556d43aac99abcaa69c1244403d29a2b9e726e8c8a6a9cd68acce490c34ce8e2d3915c6204d35f738173ccd3814521d557912b6f85d32686c77862d396791d9dbc0a4da99679bd5475363ce94a377a2854d726e9bed67905adc34bbeb05b6fb747bbdff04d130d3d4d7060000"
	decoded, err := hex.DecodeString(dumpHex[2:])

	if err != nil {
		panic(err)
	}

	fmt.Println("decoded: ", decoded)

	zipReader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		panic(err)
	}
	defer zipReader.Close()

	unzipped, err := io.ReadAll(zipReader)
	if err != nil {
		panic(err)
	}

	var dump state.Dump
	err = json.Unmarshal(unzipped, &dump)
	if err != nil {
		return fmt.Errorf("unmarshal dump: %v", err)
	}

	dump.OnAccount(&deployerAddr, state.DumpAccount{
		Balance: balance.String(),
	})

	l1genesis, err := opgenesis.BuildL1DeveloperGenesis(deployConfig, &dump, l1Deployments)
	if err != nil {
		return fmt.Errorf("build l1 developer genesis: %v", err)
	}

	l1client, l1HTTPendpoint := ethdevnet(ctx, deployConfig.L1ChainID, uint64(s.l1BlockTime.Seconds()), l1genesis)

	l1url, err := url.New(l1HTTPendpoint)

	if err != nil {
		return fmt.Errorf("new l1 url: %v", err)
	}


	// url := l1client.

	// NOTE: should we set a timeout on the context? Might not be worth the complexity.
	// if !s.anvilURL.IsReachable(ctx) {
	// 	return nil
	// }

	l1 := NewL1Client(l1client)

	fmt.Println("running forge cmd for L1 at ", l1HTTPendpoint)
	fmt.Println("prior rpc-url: ", s.anvilURL)
	fmt.Println("contractsRoot: ", s.contractsRootDir)

	// Deploy the OP L1 contracts.
	forgeCmd := exec.CommandContext( //nolint:gosec
		ctx,
		"forge",
		"script",
		"--root", s.contractsRootDir,
		"-vvv",
		fmt.Sprintf("%s:Deploy", filepath.Join(s.contractsRootDir, "scripts", "Deploy.s.sol")),
		"--rpc-url", l1HTTPendpoint,
		"--broadcast",
		"--private-key", common.Bytes2Hex(crypto.FromECDSA(deployerKey)),
	)
	if err := s.startCmd(forgeCmd); err != nil {
		return err
	}
	if err := forgeCmd.Wait(); err != nil {
		return fmt.Errorf("run %s: %v", forgeCmd, err)
	}
	fmt.Println("forge cmd done")

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

func (s *Stack) startCmd(cmd *exec.Cmd) error {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("get stdout pipe for %s: %v", cmd, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("get stderr pipe for %s: %v", cmd, err)
	}
	s.eventListener.HandleCmdOutput(cmd.Path, stdout, stderr)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %v", cmd, err)
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
