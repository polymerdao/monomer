package e2e

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
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

	zipReader, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		panic(err)
	}
	defer zipReader.Close()

	unzipped, err := io.ReadAll(zipReader)
	if err != nil {
		panic(err)
	}

	fmt.Println("unzipped: ", string(unzipped))

	{

		// var fDump opgenesis.ForgeDump

		// err = fDump.UnmarshalJSON(unzipped)
		// if err != nil {
		// 	panic(err)
		// }

		// fmt.Println("fDump: ", fDump)
	}
	var dump state.Dump
	err = json.Unmarshal(unzipped, &dump)
	if err != nil {
		panic(err)
	}
	fmt.Println("dump: ", dump)

	// prints:

	/*

		result:  0x1f8b08000000000000ffad95db6a1c310c86df65ae7b211f74705e6691643b2cdd43d8dd94949077af66935e044a68e97860f0c896be5fb6c67e5dec70f6efcbc3eb727a3edab82c0f0bbcc8f26df1f3fe647a1d7703fc650bbfdbfe38ae373d3edd1d89c811a7c5c0a35e7787fd717fbb0f246f5e6475582173bc73129324a014e6be9f73efcf87dbcf7705617aba8c1f173d753dff93a82fc446f2b61b2f3eaed7ddaa2f82ef9e2e7b1feb827cd8ef73627079800f87f8f89894dedebe2dea7e7e3eddaeab4fa450bb736b54bb6607b15ca889955edcd9b5e4d44a6f0df1bee4e7d31a658dab07bdf723424e23b501de4c2dd7df52fddcdf87a37fbd9d2ffab88a5cf9c1c45eaaeac49a89bb71672fc4525ad83829404821a54d9991579221699489de2707dc4b278c9e39a84db4082a734b66f15abdf76ea40d606a3614ec3db708537aca536b6ec57c4b661dd8ac56a96c851bb2a048cbe0ca326d6672b0da90be607e22f0fcbf168912240add12ef0c920b0b14941cbd3a5120614282b24a4d92674793988519617dc2b986b7cff2875c195ae3c6e09824e5ee45218173876408037ae2b5b65db65cdf06939b4d1a96bd4ee1c80f47ac72933cd24c9046ec6803da94c934942b641a9c09b14645214fc5ca54d43aac99abb64d992d40c03d29a2b9676ae828a68966145697d40c34ce8e2d990a63b0a69a7b9c0b9e661c0aa98eaa3219297e970c1adb9d614be62c6d761a9854bbc8a4597d909a48e61cb513256c2d67da6c3f036a71e1eceef7d8eed325f6f60bd669cd31de060000
		decoded:  [31 139 8 0 0 0 0 0 0 255 173 149 219 106 28 49 12 134 223 101 174 123 33 31 116 112 94 102 145 100 59 44 221 67 216 221 148 148 144 119 175 102 147 94 4 74 104 233 120 96 240 200 150 190 95 182 198 126 93 236 112 246 239 203 195 235 114 122 62 218 184 44 15 11 188 200 242 109 241 243 254 100 122 29 119 3 252 101 11 191 219 254 56 174 55 61 62 221 29 137 200 17 167 197 192 163 94 119 135 253 113 127 187 15 36 111 94 100 117 88 33 115 188 115 18 147 36 160 20 230 190 159 115 239 207 135 219 207 119 5 97 122 186 140 31 23 61 117 61 255 147 168 47 196 70 242 182 27 47 62 174 215 221 170 47 130 239 158 46 123 31 235 130 124 216 239 115 98 112 121 128 15 135 248 248 152 148 222 222 190 45 234 126 126 62 221 174 171 79 164 80 187 115 107 84 187 102 7 177 92 168 137 149 94 220 217 181 228 212 74 111 13 241 190 228 231 211 26 101 141 171 7 189 247 35 66 78 35 181 1 222 76 45 215 223 82 253 220 223 135 163 127 189 157 47 250 184 138 92 249 193 196 94 170 234 196 154 137 187 113 103 47 196 82 90 216 56 41 64 72 33 165 77 153 145 87 146 33 105 148 137 222 39 7 220 75 39 140 158 57 168 77 180 8 42 115 75 102 241 90 189 247 110 164 13 96 106 54 20 236 61 183 8 83 122 202 83 107 110 197 124 75 102 29 216 172 86 169 108 133 27 178 160 72 203 224 202 50 109 102 114 176 218 144 190 96 126 34 240 252 191 22 137 18 36 10 221 18 239 12 146 11 11 20 148 28 189 58 81 32 97 66 130 178 74 77 146 103 71 147 152 133 25 97 125 194 185 134 183 207 242 135 92 25 90 227 198 224 152 36 229 238 69 33 129 115 135 100 8 3 122 226 181 182 93 182 92 223 6 147 155 77 26 150 189 78 225 200 15 71 172 114 147 60 210 76 144 70 236 104 3 218 148 201 52 148 43 100 26 156 9 177 70 69 33 79 197 202 84 212 58 172 153 171 182 77 153 45 64 192 61 41 162 185 103 106 232 40 166 137 102 20 86 151 212 12 52 206 142 45 153 10 99 176 166 154 123 156 11 158 102 28 10 169 142 170 50 25 41 126 151 12 26 219 157 97 75 230 44 109 118 26 152 84 187 200 164 89 125 144 154 72 230 28 181 19 37 108 45 103 218 108 63 3 106 113 225 236 238 247 216 238 211 37 246 246 11 214 105 205 49 222 6 0 0]
		unzipped:  {"block":{"number":"0x8","coinbase":"0x0000000000000000000000000000000000000000","timestamp":"0x666c55fb","gas_limit":"0x1c9c380","basefee":"0x17681061","difficulty":"0x0","prevrandao":"0x0000000000000000000000000000000000000000000000000000000000000000","blob_excess_gas_and_price":{"excess_blob_gas":0,"blob_gasprice":1}},"accounts":{"0x14dc79964da2c08b23698b3d3cc7ca32193d9955":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x15d34aaf54267db7d7c367839aaf71a00a2c6a65":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x23618e81e3f5cdf7f54c3d65f7fbc0abf5b21e8f":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x3c44cdddb6a900fa2b585dd299e03d12fa4293bc":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x4e59b44847b379578588920ca78fbf26c0b4956c":{"nonce":0,"balance":"0x0","code":"0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe03601600081602082378035828234f58015156039578182fd5b8082525050506014600cf3","storage":{}},"0x70997970c51812dc3a010c7d01b50e0d17dc79c8":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x90f79bf6eb2c4f870365e785982e1f101e93b906":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x976ea74026e726554db657fa54763abd0c3a0aa9":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0x9965507d1a55bcc2695c58ba16fb37d819b0a4dc":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0xa0ee7a142d267c1f36714e4a8f75612f20a79720":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}},"0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266":{"nonce":0,"balance":"0x21e19e0c9bab2400000","code":"0x","storage":{}}},"best_block_number":"0x8"}
		dump:  { map[0x14dc79964da2c08b23698b3d3cc7ca32193d9955:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x15d34aaf54267db7d7c367839aaf71a00a2c6a65:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x23618e81e3f5cdf7f54c3d65f7fbc0abf5b21e8f:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x3c44cdddb6a900fa2b585dd299e03d12fa4293bc:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x4e59b44847b379578588920ca78fbf26c0b4956c:{0x0 0 0x 0x 0x7fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffe03601600081602082378035828234f58015156039578182fd5b8082525050506014600cf3 map[] <nil> 0x} 0x70997970c51812dc3a010c7d01b50e0d17dc79c8:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x90f79bf6eb2c4f870365e785982e1f101e93b906:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x976ea74026e726554db657fa54763abd0c3a0aa9:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0x9965507d1a55bcc2695c58ba16fb37d819b0a4dc:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0xa0ee7a142d267c1f36714e4a8f75612f20a79720:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x} 0xf39fd6e51aad88f6f4ce6ab8827279cfffb92266:{0x21e19e0c9bab2400000 0 0x 0x 0x map[] <nil> 0x}] []}
	*/os.Exit(0)

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
