package e2e

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	cometdb "github.com/cometbft/cometbft-db"
	dbm "github.com/cosmos/cosmos-db"
	opgenesis "github.com/ethereum-optimism/optimism/op-chain-ops/genesis"
	"github.com/ethereum/go-ethereum/common"
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
	// Run anvil.
	anvilCmd := exec.CommandContext( //nolint:gosec
		ctx,
		"anvil",
		"--port", s.anvilURL.Port(),
		"--order", "fifo",
		"--disable-block-gas-limit",
		"--gas-price", "0",
		"--block-time", fmt.Sprint(s.l1BlockTime.Seconds()),
	)
	anvilCmd.Cancel = func() error {
		// Anvil can catch SIGTERMs. The exec package sends a SIGKILL by default.
		return anvilCmd.Process.Signal(syscall.SIGTERM)
	}
	if err := s.startCmd(anvilCmd); err != nil {
		return err
	}
	env.Go(func() {
		if err := anvilCmd.Wait(); err != nil && !errors.Is(err, ctx.Err()) {
			s.eventListener.OnAnvilErr(fmt.Errorf("run %s: %v", anvilCmd, err))
		}
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

	var result string
	err = anvil.client.CallContext(ctx, &result, "anvil_dumpState")
	if err != nil {
		return fmt.Errorf("dump state: %v", err)
	}

	fmt.Println("result: ", result)
	decoded, err := hex.DecodeString(result[2:])
	if err != nil {
		panic(err)
	}
	fmt.Println("decoded: ", decoded)

	// prints:

	/*

			result:  0x1f8b08000000000000ffad55db6a1c310cfd9779ce837c9125e5671649b6c3d26437ec6e4a4ac8bf57b39b160225b4743c307864e99c235963bf2df678f46fcbf
		ddb727879b2715aee177885e56ef1e3fe607a1e37c35f8e88bbec9fc6f9a24fcfd7c01496073def1ef74ffbcbcde2e28557cf157d8e1b413151d72b40dfcfb9f797c7cb8fdf5
		a9e4fe3fb490f5d8fffa4e60b9591b5edc6ab8ff379b7ea0bf0ddf369ef63adc487fdea138bcb3d7c04c4c787537a7fbf5bd4fdf872b89cd79848ac76279156bb6607b65c9ab
		0955edcc9b5e424a58b205e6b7d3cac282bae3eea751e08398d24035c4c2dd75f52fdd86fcb313f5f8e277d5845aefcc189bd54d5893537ea469dbc34e22261a314f50c294dd
		ba69c9157e2c1699489de2705b997de3066e6a036d10294e7969cc56bf5debb351580a9d990b1f72c01537aca536b9662be25671d28562b57b24282c4c82c195c89a7cddc1ca
		c0ab62f383f31d0fcbf118936482d7473bc33702ec4509073ccea448684091b94556ae23c3b1a87176684f589e01ad13ecb1f7225102121704c9c72f7a290c0a9433284013dd
		1dadbce5bd6576092d86cc3b2d7c914f9e1882a0be791668234624705daa69cd4865285dc06e58658a3a390a662a556d43aac99abcaa69c1244403d29a2b9e726e8c8a6a9cd6
		8acce490c34ce8e2d3915c6204d35f738173ccd3814521d557912b6f85d32686c77862d396791d9dbc0a4da99679bd5475363ce94a377a2854d726e9bed67905adc34bbeb05b
		6fb747bbdff04d130d3d4d7060000

		decoded:  [31 139 8 0 0 0 0 0 0 255 173 85 219 106 28 49 12 253 151 121 206 131 124 145 37 229 103 22 73 182 195 210 100 55 236 110 74 74 20
		0 191 87 179 155 22 2 37 180 116 60 48 120 100 233 156 35 89 99 191 45 246 120 244 111 203 253 219 114 120 121 178 113 90 238 23 120 133 229
		 110 241 227 254 96 122 30 55 195 95 142 136 187 236 159 198 249 162 79 207 215 192 20 150 7 61 239 30 247 79 251 203 205 226 226 133 87 207
		 21 125 142 27 65 49 81 215 43 64 223 207 185 247 151 199 203 143 223 90 158 79 227 251 73 15 93 143 255 164 230 11 149 145 181 237 198 171
		143 243 121 183 234 11 240 221 243 105 239 99 173 196 135 253 234 19 139 203 61 124 4 196 199 135 83 122 127 191 91 212 253 248 114 184 156
		215 152 72 172 118 39 145 86 187 102 7 182 92 154 176 149 94 220 201 181 228 36 165 139 32 94 107 125 60 172 40 43 174 62 234 117 30 8 57 14
		1 36 3 92 76 45 215 95 82 253 216 111 203 49 63 95 142 39 125 88 69 174 252 193 137 189 84 213 137 53 55 234 70 157 188 52 226 34 97 163 20
		245 12 41 77 219 166 156 145 87 226 193 105 148 137 222 39 5 185 151 222 48 102 230 160 54 209 2 148 231 150 156 197 107 245 222 187 53 21 1
		28 169 217 144 177 247 44 1 83 122 202 83 107 150 98 190 37 103 29 40 86 43 87 178 66 130 196 200 44 25 92 137 167 205 220 28 172 10 182 47
		56 63 49 208 252 191 17 137 54 72 45 116 115 188 51 112 46 196 80 144 115 204 234 68 134 132 9 27 148 85 106 226 60 59 26 135 23 102 132 245
		 137 224 26 209 62 203 31 114 37 16 33 33 112 76 156 114 247 162 144 192 169 67 50 132 1 61 209 218 219 206 91 214 87 96 146 216 108 195 178
		 215 201 20 249 225 136 42 11 231 145 102 130 52 98 71 5 218 166 156 212 134 82 133 220 6 229 134 88 163 163 144 166 98 165 86 212 58 172 15
		3 171 202 166 156 18 68 64 61 41 162 185 231 38 232 200 166 169 205 104 172 206 73 12 52 206 142 45 57 21 198 32 77 53 247 56 23 60 205 56 2
		0 82 29 85 121 18 182 248 93 50 104 108 119 134 45 57 103 145 217 219 192 164 218 153 103 155 213 71 83 99 206 148 163 119 162 133 77 114 11
		0 155 237 103 144 90 220 52 187 235 5 182 251 116 123 189 255 4 209 48 211 212 215 6 0 0]

	*/

	os.Exit(0)

	privKey, err := crypto.GenerateKey()
	if err != nil {
		return fmt.Errorf("generate key: %v", err)
	}
	if err := anvil.SetBalance(ctx, crypto.PubkeyToAddress(privKey.PublicKey), 10*oneETH); err != nil { //nolint:gomnd
		return fmt.Errorf("set balance: %v", err)
	}

	// Deploy the OP L1 contracts.
	forgeCmd := exec.CommandContext( //nolint:gosec
		ctx,
		"forge",
		"script",
		"--root", s.contractsRootDir,
		"-vvv",
		fmt.Sprintf("%s:Deploy", filepath.Join(s.contractsRootDir, "scripts", "Deploy.s.sol")),
		"--rpc-url", s.anvilURL.String(),
		"--broadcast",
		"--private-key", common.Bytes2Hex(crypto.FromECDSA(privKey)),
	)
	if err := s.startCmd(forgeCmd); err != nil {
		return err
	}
	if err := forgeCmd.Wait(); err != nil {
		return fmt.Errorf("run %s: %v", forgeCmd, err)
	}
	latestL1Block, err := anvil.BlockByNumber(ctx, nil)
	if err != nil {
		return fmt.Errorf("get the latest l1 block: %v", err)
	}

	// Run Monomer.
	const l2ChainID = 901
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
