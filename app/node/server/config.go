package server

import (
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	cmdb "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/spf13/cobra"
)

type DbBackendType = cmdb.BackendType

const GoLevelDBBackend = cmdb.GoLevelDBBackend

var (
	DefaultNodeHome         string
	SupportedDbBackendTypes = []string{
		string(cmdb.GoLevelDBBackend), string(cmdb.CLevelDBBackend),
		string(cmdb.RocksDBBackend), string(cmdb.BadgerDBBackend), string(cmdb.BoltDBBackend),
		string(cmdb.MemDBBackend),
	}
)

type ApiEnabledMask uint64

func EnableAllApis() ApiEnabledMask {
	return ^ApiEnabledMask(0)
}

func (a ApiEnabledMask) IsAdminApiEnabled() bool {
	return a&adminApiEnabled > 0
}

const (
	adminApiEnabled = 1 << 0
	// PeptideApiEnabled = 1 << 1
	// EthApiEnabled = 1 << 2
)

const (
	AppStateStore = "app"
	BlockStore    = "block"
	TxStore       = "tx"
	PayloadStore  = "payload"
)

const (
	homedirFlag                 = "home"
	blockTimeFlag               = "block-time"
	abciServerAddressFlag       = "abci-server-address"
	appGrpcAddressFlag          = "app-grpc-address"
	appRpcAddressFlag           = "app-rpc-address"
	transportFlag               = "transport"
	eeRpcAddressFlag            = "ee-http-server-address"
	dbBackendFlag               = "db-backend"
	cpuProfileFlag              = "cpu-profile"
	pprofRpcAddrFlag            = "pprof-address"
	chainIdFlag                 = "chain-id"
	l1HashFlag                  = "l1-hash"
	l1HeightFlag                = "l1-height"
	outputFlag                  = "output"
	overrideFlag                = "override"
	genesisTimeFlag             = "genesis-time"
	prometheusRetentionTimeFlag = "prometheus-retention-time"
	adminFlag                   = "admin-api"
	blocksFlag                  = "blocks"
	rollbackBlocksFlag          = "rollback-blocks"
	rollbackStore               = "store"
	iavlDisableFastNodeFlag     = "iavl-disable-fastnode"
	iavlLazyLoadingFlag         = "iavl-lazy-loading"
)

type Config struct {
	cmd                     *cobra.Command
	HomeDir                 string
	AbciServerRpc           Endpoint
	AbciServerGrpc          Endpoint
	PeptideCometServerRpc   Endpoint
	PeptideEngineServerRpc  Endpoint
	GenesisConfig           rollup.Genesis
	GenesisTime             time.Time
	CpuProfile              string
	PprofRpc                Endpoint
	DbBackend               DbBackendType
	Logger                  Logger
	L1                      eth.BlockID
	Override                bool
	ChainId                 string
	Output                  *os.File
	BlockTime               time.Duration
	PrometheusRetentionTime int64
	Apis                    ApiEnabledMask
	BlocksInStore           int64
	RollbackBlocks          int64
	RollbackStore           string
	IavlDisableFastNode     bool
	IavlLazyLoading         bool
}

// TODO load config file here and use it as the base config that can be overwritten by cli options
func NewConfig(cmd *cobra.Command) *Config {
	return &Config{cmd: cmd}
}

func (c *Config) WithOverride() *Config {
	c.Override = c.mustReadBoolFlag(overrideFlag)
	return c
}

func (c *Config) WithHomeDir() *Config {
	c.HomeDir = c.mustReadStringFlag(homedirFlag)
	return c
}

func (c *Config) WithAbciServerRpc() *Config {
	c.AbciServerRpc = NewEndpoint(c.mustReadStringFlag(abciServerAddressFlag))
	return c
}

func (c *Config) WithAbciServerGrpc() *Config {
	c.AbciServerGrpc = NewEndpoint(c.mustReadStringFlag(appGrpcAddressFlag))
	return c
}

func (c *Config) WithAdminApi() *Config {
	if c.mustReadBoolFlag(adminFlag) {
		c.Apis |= adminApiEnabled
	}
	return c
}

func (c *Config) WithPeptideCometServerRpc() *Config {
	c.PeptideCometServerRpc = NewEndpoint(c.mustReadStringFlag(appRpcAddressFlag))
	return c
}

func (c *Config) WithPeptideEngineServerRpc() *Config {
	c.PeptideEngineServerRpc = NewEndpoint(c.mustReadStringFlag(eeRpcAddressFlag))
	return c
}

func (c *Config) WithGenesisConfig(genesisConfig rollup.Genesis) *Config {
	c.GenesisConfig = genesisConfig
	return c
}

func (c *Config) WithGenesisTime() *Config {
	t := c.mustReadIntFlag(genesisTimeFlag)
	c.GenesisTime = time.Unix(t, 0)
	return c
}

func (c *Config) WithDbBackend() *Config {
	dbBackend := c.mustReadStringFlag(dbBackendFlag)
	for _, supportedType := range SupportedDbBackendTypes {
		if dbBackend == supportedType {
			c.DbBackend = cmdb.BackendType(dbBackend)
			return c
		}
	}
	log.Fatalf("invalid DbBackend: %s", c.DbBackend)
	return nil
}

func (c *Config) WithLogger(logger Logger) *Config {
	c.Logger = logger
	return c
}

func (c *Config) WithChainId() *Config {
	c.ChainId = c.mustReadStringFlag(chainIdFlag)
	return c
}

func (c *Config) WithPrometheusRetentionTime() *Config {
	c.PrometheusRetentionTime = c.mustReadIntFlag(prometheusRetentionTimeFlag)
	return c
}

func (c *Config) WithL1() *Config {
	l1Hash := c.mustReadStringFlag(l1HashFlag)
	l1Height := c.mustReadStringFlag(l1HeightFlag)
	hash, err := hexutil.Decode(l1Hash)
	if err != nil {
		log.Fatalf("invalid l1 hash: %s", l1Hash)
	}
	var height uint64
	// expect a decimal or hex height
	if h, err := strconv.ParseUint(l1Height, 10, 64); err == nil {
		height = h
	} else if h, err := strconv.ParseUint(l1Height, 16, 64); err == nil {
		height = h
	} else {
		log.Fatalf("invalid l1 height: %s", l1Hash)
	}
	c.L1.Number = height
	copy(c.L1.Hash[:], hash)
	return c
}

func (c *Config) WithOuput() *Config {
	outputFile := c.mustReadStringFlag(outputFlag)
	if outputFile == "" || outputFile == "-" {
		c.Output = os.Stdout
	} else {
		output, err := os.Create(outputFile)
		if err != nil {
			log.Fatalf("%v", err)
		}
		c.Output = output
	}
	return c
}

func (c *Config) WithBlockTime() *Config {
	t := c.mustReadIntFlag(blockTimeFlag)
	c.BlockTime = time.Duration(t) * time.Millisecond
	return c
}

func (c *Config) WithRollbackStore() *Config {
	s := c.mustReadStringFlag(rollbackStore)
	if s != BlockStore && s != AppStateStore && s != TxStore && s != PayloadStore {
		log.Fatalf("invalid store name: '%s'", s)
	}
	c.RollbackStore = s
	return c
}

func (c *Config) WithBlocks() *Config {
	b := c.mustReadIntFlag(blocksFlag)
	if b <= 0 {
		log.Fatalf("invalid number of blocks: %d", b)
	}
	c.BlocksInStore = b
	return c
}

func (c *Config) WithRollbackBlocks() *Config {
	b := c.mustReadIntFlag(rollbackBlocksFlag)
	if b <= 0 {
		log.Fatalf("invalid number of blocks to rollback: %d", b)
	}
	c.RollbackBlocks = b
	return c
}

func (c *Config) WithCpuProfile() *Config {
	c.CpuProfile = c.mustReadStringFlag(cpuProfileFlag)
	return c
}

func (c *Config) WithPprofRpc() *Config {
	c.PprofRpc = NewEndpoint(c.mustReadStringFlag(pprofRpcAddrFlag))
	return c
}

func (c *Config) WithIavlDisableFastNode() *Config {
	c.IavlDisableFastNode = c.mustReadBoolFlag(iavlDisableFastNodeFlag)
	return c
}

func (c *Config) WithIavlLazyLoading() *Config {
	c.IavlLazyLoading = c.mustReadBoolFlag(iavlLazyLoadingFlag)
	return c
}

func AddExportCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	cmd.Flags().StringP(outputFlag,
		"o",
		"",
		"Output file (default - to stdout)",
	)
}

func AddInitCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	cmd.Flags().String(l1HashFlag,
		"",
		"The L1 block hash that the rollup starts *after*",
	)
	cmd.Flags().String(l1HeightFlag,
		"",
		"The L1 block height (dec or hex) that the rollup starts *after*",
	)
	cmd.Flags().String(chainIdFlag,
		"901",
		"genesis file chain-id",
	)
}

func AddSealCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	cmd.Flags().Int64(genesisTimeFlag,
		time.Now().Unix(),
		"Timestamp to be used within the genesis block in seconds since Epoch. Defaults to right now",
	)
}

func AddIavlCommandFlags(cmd *cobra.Command) {
	cmd.Flags().Bool(
		iavlDisableFastNodeFlag,
		true,
		"Disable fast node for IAVL tree",
	)
	cmd.Flags().Bool(
		iavlLazyLoadingFlag,
		true,
		"Enable lazy loading for IAVL tree",
	)
}

func AddCpuProfileCommandFlags(cmd *cobra.Command) {
	cmd.Flags().String(
		cpuProfileFlag,
		"",
		"Enable CPU profiling and write to the provided file",
	)
	cmd.Flags().String(
		pprofRpcAddrFlag,
		"localhost:6060",
		"pprof listen address (https://golang.org/pkg/net/http/pprof)",
	)
}

func AddStartCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	AddCpuProfileCommandFlags(cmd)
	AddIavlCommandFlags(cmd)
	cmd.Flags().String(appRpcAddressFlag,
		"localhost:26657",
		"Address of the JSON-RPC app server. Set - to disable.",
	)
	cmd.Flags().String(abciServerAddressFlag,
		"-",
		"Address to listen on. Set - to disable. Eg. tcp://localhost:26658, unix:///tmp/polymer.sock",
	)
	cmd.Flags().String(transportFlag,
		"socket",
		"ABCI Server Transport type (socket or grpc)",
	)
	cmd.Flags().String(appGrpcAddressFlag,
		"-",
		"Address of the gRPC app server. Set - to disable. Eg. tcp://localhost:9090",
	)
	cmd.Flags().String(
		eeRpcAddressFlag,
		"localhost:8545",
		"Address of the Execution Engine JSON-RPC HTTP endpoint. Set - to disable.",
	)
	cmd.Flags().Int64(
		prometheusRetentionTimeFlag,
		0,
		"Prometheus retention time in seconds. 0 means prometheus sink is disabled",
	)
	cmd.Flags().Bool(
		adminFlag,
		false,
		"If set, it enables the admin API",
	)
}

func AddStandaloneCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	AddCpuProfileCommandFlags(cmd)
	AddIavlCommandFlags(cmd)
	cmd.Flags().String(
		appRpcAddressFlag,
		"localhost:26657",
		"Address of the JSON-RPC app server. Set - to disable.",
	)
	cmd.Flags().String(
		eeRpcAddressFlag,
		"localhost:8545",
		"Address of the Execution Engine JSON-RPC HTTP endpoint. Set - to disable.",
	)
	cmd.Flags().Int64(
		blockTimeFlag,
		2000,
		"Block time in milliseconds. Defaults to: 2000ms",
	)
	cmd.Flags().String(chainIdFlag,
		"901",
		"genesis file chain-id",
	)
	cmd.Flags().Int64(
		prometheusRetentionTimeFlag,
		0,
		"Prometheus retention time in seconds. 0 means prometheus sink is disabled",
	)
}

func AddProfileRollbackCommandFlags(cmd *cobra.Command) {
	AddDefaultFlags(cmd)
	AddCpuProfileCommandFlags(cmd)
	AddIavlCommandFlags(cmd)
	cmd.Flags().Int64(
		blocksFlag,
		10,
		"Number of blocks in the store before starting the rollback",
	)
	cmd.Flags().Int64(
		rollbackBlocksFlag,
		5,
		"Number of blocks to rollback",
	)
	stores := strings.Join([]string{AppStateStore, BlockStore, TxStore, PayloadStore}, ", ")
	cmd.Flags().String(
		rollbackStore,
		"",
		"Store to rollback. Choose from: "+stores,
	)
}

func (c *Config) mustReadStringFlag(flag string) string {
	v, err := c.cmd.Flags().GetString(flag)
	if err != nil {
		log.Fatalf("error reading flag '%s': %v", flag, err)
	}
	return v
}

func (c *Config) mustReadIntFlag(flag string) int64 {
	v, err := c.cmd.Flags().GetInt64(flag)
	if err != nil {
		log.Fatalf("error reading flag '%s': %v", flag, err)
	}
	return v
}

func (c *Config) mustReadBoolFlag(flag string) bool {
	v, err := c.cmd.Flags().GetBool(flag)
	if err != nil {
		log.Fatalf("error reading flag '%s': %v", flag, err)
	}
	return v
}

func AddDefaultFlags(cmd *cobra.Command) {
	cmd.Flags().String(dbBackendFlag,
		string(GoLevelDBBackend),
		"Database backend type. Supported types: "+strings.Join(SupportedDbBackendTypes, ", "),
	)
	cmd.Flags().String(homedirFlag,
		DefaultNodeHome,
		"Home directory. Defaults to: "+DefaultNodeHome,
	)
	cmd.Flags().Bool(
		overrideFlag,
		false,
		"Overrides any existing storage in the homedir",
	)
}

func init() {
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	DefaultNodeHome = filepath.Join(userHomeDir, ".peptide")
}
