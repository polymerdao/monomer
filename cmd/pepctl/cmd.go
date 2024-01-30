package main

import (
	"fmt"

	tmdb "github.com/cometbft/cometbft-db"
	tmlog "github.com/cometbft/cometbft/libs/log"
	rms "github.com/cosmos/cosmos-sdk/store/rootmulti"
	gogotypes "github.com/cosmos/gogoproto/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer/app/node"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/app/peptide/store"
	"github.com/polymerdao/monomer/app/peptide/txstore"
	"github.com/spf13/cobra"
)

func RootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "pepctl",
		Short: "pepctl manages Peptide node resources",
	}
	rootCmd.AddCommand(dbCmd())
	rootCmd.AddCommand(standaloneCmd())
	rootCmd.AddCommand(profileCmd())
	return rootCmd
}

// appCreator defines a func that return a *peptide.PeptideApp
type AppCreator func(chainID, homedir string, db tmdb.DB, logger tmlog.Logger) *peptide.PeptideApp

var DefaultAppCreator = peptide.New

func dbCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Peptide db stores",
	}
	cmd.AddCommand(dbInspectCmd())
	cmd.AddCommand(dbStoreRollbackCmd())
	cmd.AddCommand(dbResetAppCmd())
	return cmd
}

func dbInspectCmd() *cobra.Command {
	inspect := func(stores DbStores) error {
		bs, _, app, genesis, config, logger := stores.bs, stores.txStore, stores.app, stores.genesis, stores.config, stores.logger
		logger.Info("Peptide db inspect", "home", config.HomeDir, "chain-id", genesis.ChainID, "db-backend", config.DbBackend, "db-override", config.Override)

		logger.Info("Genesis", "error", genesis.Validate(), "time", genesis.GenesisTime, "initalHeight", genesis.InitialHeight)
		logger.Info("GenesisBlock", "height", genesis.GenesisBlock.Number, "hash", genesis.GenesisBlock.Hash)
		logger.Info("Genesis L1", "height", genesis.L1.Number, "hash", genesis.L1.Hash)

		logger.Info("App state", "appLastBlockHeight", app.LastBlockHeight(), "ChainId", app.ChainId, "BondDenom", app.BondDenom)
		logger.Info("Block store at app.lastBlockHeight()",
			"blockExists", bs.BlockByNumber(app.LastBlockHeight()) != nil,
			"hash", getBlockHash(bs.BlockByNumber(app.LastBlockHeight())),
		)
		// use RollbackSettings to validate and verify the block store
		setting := RollbackSettings{
			height:          getBlockHeight(bs.BlockByLabel(eth.Unsafe)),
			unsafeHeight:    getBlockHeight(bs.BlockByLabel(eth.Unsafe)),
			safeHeight:      getBlockHeight(bs.BlockByLabel(eth.Safe)),
			finalizedHeight: getBlockHeight(bs.BlockByLabel(eth.Finalized)),
		}
		logger.Info("Block store height by labels",
			"latest/unsafe", setting.unsafeHeight,
			"safe", setting.safeHeight,
			"finalized", setting.finalizedHeight,
			"appLastBlockHeight", app.LastBlockHeight(),
		)
		if err := setting.Validate(); err != nil {
			logger.Error("Stores sanity check", "error", err)
			return err
		}
		if err := setting.Verify(app, bs); err != nil {
			logger.Error("Stores sanity check", "error", err)
			return err
		}
		logger.Info("Stores sanity check success")

		return nil
	}

	cmd := &cobra.Command{
		Use:     "inspect",
		Aliases: []string{"i"},
		Short:   "Inspect Peptide db",
		RunE: func(cmd *cobra.Command, args []string) error {
			return performDbOps(cmd, inspect)
		},
	}

	server.AddDefaultFlags(cmd)
	return cmd
}

func dbStoreRollbackCmd() *cobra.Command {
	setting := RollbackSettings{}

	rollback := func(stores DbStores) error {
		bs, _, app, genesis, config, logger := stores.bs, stores.txStore, stores.app, stores.genesis, stores.config, stores.logger
		logger.Info("Peptide db rollback", "home", config.HomeDir, "chain-id", genesis.ChainID, "db-backend", config.DbBackend, "db-override", config.Override)
		if !config.Override { // sanity check
			return fmt.Errorf("--override must be set for write operations")
		}
		logger.Info("RollbackSetting", "rollbackHeight", setting.height, "unsafeHeight", setting.unsafeHeight, "safeHeight", setting.safeHeight, "finalizedHeight", setting.finalizedHeight)
		if err := setting.Validate(); err != nil {
			logger.Error("RollbackSetting", "error", err)
			return err
		}

		if err := setting.Verify(app, bs); err != nil {
			logger.Error("RollbackSetting", "error", err)
			return err
		}

		if setting.dryRun != nil && *setting.dryRun {
			logger.Info("Rollback setting OK. Dry run success")
			return nil
		}

		// begin rollback; any err may cause data corruption

		if err := app.RollbackToHeight(*setting.height); err != nil {
			logger.Error("App RollbackToHeight", "height", setting.height, "error", err)
			return err
		}

		if err := bs.RollbackToHeight(*setting.height); err != nil {
			logger.Error("BlockStore RollbackToHeight", "height", setting.height, "error", err)
			return err
		}
		// update 3 labels
		if err := bs.UpdateLabel(eth.Unsafe, bs.BlockByNumber(*setting.unsafeHeight).Hash()); err != nil {
			logger.Error("BlockStore UpdateLabel", "label", eth.Unsafe, "height", setting.unsafeHeight, "error", err)
			return err
		}
		if err := bs.UpdateLabel(eth.Safe, bs.BlockByNumber(*setting.safeHeight).Hash()); err != nil {
			logger.Error("BlockStore UpdateLabel", "label", eth.Safe, "height", setting.safeHeight, "error", err)
			return err
		}
		if err := bs.UpdateLabel(eth.Finalized, bs.BlockByNumber(*setting.finalizedHeight).Hash()); err != nil {
			logger.Error("BlockStore UpdateLabel", "label", eth.Finalized, "height", setting.finalizedHeight, "error", err)
			return err
		}

		logger.Info("Rollback success")
		logger.Info("Block store height by labels",
			"latest/unsafe", setting.unsafeHeight,
			"safe", setting.safeHeight,
			"finalized", setting.finalizedHeight,
			"appLastBlockHeight", app.LastBlockHeight(),
		)

		return nil
	}

	cmd := &cobra.Command{
		Use:     "rollback",
		Short:   "Rollback stores to a given height with integrity check",
		Long:    "Precondition: Chain App state is not corrupted. If App is corrupted, try to force reset its latest version using `pepctl db reset-app --latest-version 1234`. ",
		Aliases: []string{"rb"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return performDbOps(cmd, rollback)
		},
	}

	server.AddDefaultFlags(cmd)
	flagRollbackHeight := "rollback-height"
	flagUnsafe := "unsafe"
	flagSafe := "safe"
	flagFinalized := "finalized"

	setting.height = cmd.Flags().Int64P(flagRollbackHeight, "r", 0, "Rollback BlockStore/AppState to height")
	setting.unsafeHeight = cmd.Flags().Int64P(flagUnsafe, "u", 0, "Block height of unsafe block (must be <= height)")
	setting.safeHeight = cmd.Flags().Int64P(flagSafe, "s", 0, "Block height of safe block (must be <= unsafe)")
	setting.finalizedHeight = cmd.Flags().Int64P(flagFinalized, "f", 0, "Block height of finalized block (must be <= safe)")
	setting.dryRun = cmd.Flags().BoolP("dry-run", "n", false, "Dry run without updating stores; check if rollback is possible")

	cmd.MarkFlagRequired(flagRollbackHeight)
	cmd.MarkFlagsRequiredTogether(flagRollbackHeight, flagUnsafe, flagSafe, flagFinalized)
	return cmd
}

const (
	latestVersionKey = "s/latest"
)

func dbResetAppCmd() *cobra.Command {
	var argLatestVersion *int64
	var dryRun *bool
	logger := server.DefaultLogger()

	cmd := &cobra.Command{
		Use:     "reset-app",
		Short:   "Reset app state to a given latest version/height (UNSAFE)",
		Long:    "NOTE: onlyuse this cmd when App state is corrupted. Otherwise try `pepctl db rollback",
		Aliases: []string{"ra"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := server.NewConfig(cmd).WithHomeDir().WithLogger(logger).WithDbBackend()
			logger.Info("Peptide db reset-app", "home", cfg.HomeDir, "latest-version", argLatestVersion, "dry-run", dryRun)

			appdb := server.OpenDB(node.AppStateDbName, cfg)
			defer appdb.Close()
			existingLatestVersion := rms.GetLatestVersion(appdb)
			logger.Info("Existing latest version", "version", existingLatestVersion)

			newVersionBz, err := gogotypes.StdInt64Marshal(*argLatestVersion)
			if err != nil {
				return fmt.Errorf("failed to marshal latest version: %v", err)
			}
			if *dryRun {
				return nil
			}

			// mutation starts
			err = appdb.Set([]byte(latestVersionKey), newVersionBz)
			if err != nil {
				return fmt.Errorf("failed to set latest version: %v", err)
			}
			logger.Info("Reset app state success", "latest-version", rms.GetLatestVersion(appdb))

			return nil
		},
	}

	server.AddDefaultFlags(cmd)
	flagLatestVersion := "latest-version"

	argLatestVersion = cmd.Flags().Int64P(flagLatestVersion, "l", 0, "Latest version of the corrupted app state")
	dryRun = cmd.Flags().BoolP("dry-run", "n", false, "Dry run mode without mutation")
	cmd.MarkFlagRequired(flagLatestVersion)
	return cmd
}

type DbStores struct {
	bs      store.BlockStore
	txStore txstore.TxStore
	app     *peptide.PeptideApp
	genesis *node.PeptideGenesis
	config  *server.Config
	logger  server.Logger
}

type RollbackSettings struct {
	height          *int64
	unsafeHeight    *int64
	safeHeight      *int64
	finalizedHeight *int64
	dryRun          *bool
}

// Validate checks if the rollback settings are valid
func (s RollbackSettings) Validate() error {
	if s.height == nil || *s.height <= 0 {
		return fmt.Errorf("rollback height must be set: %v", s.height)
	}
	if s.unsafeHeight == nil || *s.unsafeHeight <= 0 {
		return fmt.Errorf("unsafe height must be set")
	}
	if s.safeHeight == nil || *s.safeHeight <= 0 {
		return fmt.Errorf("safe height must be set")
	}
	if s.finalizedHeight == nil || *s.finalizedHeight <= 0 {
		return fmt.Errorf("finalized height must be set")
	}

	// in most cases, the rollback height should be the same as unsafe height
	// but Peptdie support recovery from a corrupted block store only if rollbackHeight == unsafeHeight + 1
	if *s.height != *s.unsafeHeight && *s.height != *s.unsafeHeight+1 {
		return fmt.Errorf("rollback height [%d] must be == unsafe height [%d] or unsafe height + 1 [%d]", *s.height, *s.unsafeHeight, *s.unsafeHeight+1)
	}

	// ensure rollbackHeight >= unsafe >= safe >= finalized
	if *s.unsafeHeight > *s.height {
		return fmt.Errorf("unsafe height [%d] must be <= rollback height [%d]", *s.unsafeHeight, *s.height)
	}
	if *s.safeHeight > *s.unsafeHeight {
		return fmt.Errorf("safe height [%d] must be <= unsafe height [%d]", *s.safeHeight, *s.unsafeHeight)
	}
	if *s.finalizedHeight > *s.safeHeight {
		return fmt.Errorf("finalized height [%d] must be <= safe height [%d]", *s.finalizedHeight, *s.safeHeight)
	}
	return nil
}

// Verify checks if the rollback settings are valid per PeptideApp and BlockStore
func (s RollbackSettings) Verify(app *peptide.PeptideApp, bs store.BlockStore) error {
	if *s.height > app.LastBlockHeight() {
		return fmt.Errorf("rollback height [%d] must be <= app.lastBlockHeight() [%d]", *s.height, app.LastBlockHeight())
	}
	if bs.BlockByNumber(app.LastBlockHeight()) == nil {
		return fmt.Errorf("app.lastBlockHeight() [%d] does not exist in blockStore", app.LastBlockHeight())
	}
	if bs.BlockByNumber(*s.height) == nil {
		return fmt.Errorf("rollback height [%d] does not exist in blockStore", *s.height)
	}
	if bs.BlockByNumber(*s.unsafeHeight) == nil {
		return fmt.Errorf("unsafe height [%d] does not exist in blockStore", *s.unsafeHeight)
	}
	if bs.BlockByNumber(*s.safeHeight) == nil {
		return fmt.Errorf("safe height [%d] does not exist in blockStore", *s.safeHeight)
	}
	if bs.BlockByNumber(*s.finalizedHeight) == nil {
		return fmt.Errorf("finalized height [%d] does not exist in blockStore", *s.finalizedHeight)
	}
	return nil
}

func performDbOps(cmd *cobra.Command, op func(stores DbStores) error) error {
	logger := server.DefaultLogger()
	config := server.NewConfig(cmd).
		WithHomeDir().
		WithDbBackend().
		WithGenesisConfig(*eetypes.NewGenesisConfig(eetypes.ZeroHash, 0)).
		WithLogger(logger).WithOverride()

	genesis, err := node.PeptideGenesisFromFile(config.HomeDir)
	if err != nil {
		return err
	}

	appdb := server.OpenDB(node.AppStateDbName, config)
	defer appdb.Close()
	app := DefaultAppCreator(genesis.ChainID, config.HomeDir, appdb, logger)

	txIndexerDb := server.OpenDB(node.TxStoreDbName, config)
	defer txIndexerDb.Close()

	bsdb := server.OpenDB(node.BlockStoreDbName, config)
	defer bsdb.Close()

	bs := store.NewBlockStore(bsdb, eetypes.BlockUnmarshaler)
	txStore := txstore.NewTxStore(txIndexerDb, logger)

	// shouldn't try to create peptide node, since Node will try to self-correct its store states
	// peptideNode, err := node.NewPeptideNodeFromConfig(app, bsdb, txIndexerDb, genesis, config)

	return op(DbStores{
		bs:      bs,
		txStore: txStore,
		app:     app,
		genesis: genesis,
		config:  config,
		logger:  logger,
	})
}

func getBlockHash(block eetypes.BlockData) string {
	if block == nil {
		return ""
	}
	return block.Hash().String()
}

func getBlockHeight(blockData eetypes.BlockData) *int64 {
	if blockData == nil {
		return nil
	}
	height := blockData.Height()
	return &height
}
