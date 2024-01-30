package main

import (
	"fmt"
	"os"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	tmlog "github.com/cometbft/cometbft/libs/log"
	"github.com/polymerdao/monomer/app/node"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/spf13/cobra"
)

func rollbackOneStore(node *node.PeptideNode, store string, rollbackHeight, latestHeight int64) error {
	switch store {
	case server.AppStateStore:
		node.RollbackAppStore(rollbackHeight)
	case server.PayloadStore:
		node.RollbackPayloadStore(rollbackHeight)
	case server.TxStore:
		node.RollbackTxStore(rollbackHeight, latestHeight)
	case server.BlockStore:
		head, err := node.GetBlock(rollbackHeight)
		if err != nil {
			return err
		}
		safe, err := node.GetBlock(rollbackHeight)
		if err != nil {
			return err
		}
		finalized, err := node.GetBlock(rollbackHeight)
		if err != nil {
			return err
		}
		node.RollbackBlockStore(head, safe, finalized)
	}
	return nil
}

func runProfileStoreRollback(cmd *cobra.Command, args []string) error {
	// use a noop logger here so we can hide the node and app output.
	config := server.NewConfig(cmd).
		WithHomeDir().
		WithLogger(tmlog.NewNopLogger()).
		WithDbBackend().
		WithCpuProfile().
		WithPprofRpc().
		WithIavlLazyLoading().
		WithIavlDisableFastNode().
		WithRollbackStore().
		WithBlocks().
		WithRollbackBlocks()

	// we can prevent the user from setting these for better ux
	config.PeptideCometServerRpc = server.NewEndpoint("localhost:26657")
	config.PeptideEngineServerRpc = server.NewEndpoint("localhost:8545")
	config.ChainId = "901"

	if err := os.RemoveAll(config.HomeDir); err != nil {
		return fmt.Errorf("failed to remove home directory: %w", err)
	}

	// only use the real db type for that store to be profiled with the intention
	// of speeding things up a bit.
	apptype, txtype, bstype := func() (server.DbBackendType, server.DbBackendType, server.DbBackendType) {
		switch config.RollbackStore {
		case server.AppStateStore:
			return config.DbBackend, tmdb.MemDBBackend, tmdb.MemDBBackend
		case server.TxStore:
			return tmdb.MemDBBackend, config.DbBackend, tmdb.MemDBBackend
		case server.BlockStore:
			return tmdb.MemDBBackend, tmdb.MemDBBackend, config.DbBackend
		}
		return tmdb.MemDBBackend, tmdb.MemDBBackend, tmdb.MemDBBackend
	}()

	appdb := server.OpenDBextended(node.AppStateDbName, config.HomeDir, apptype)
	defer appdb.Close()

	txIndexerDb := server.OpenDBextended(node.TxStoreDbName, config.HomeDir, txtype)
	defer txIndexerDb.Close()

	bsdb := server.OpenDBextended(node.BlockStoreDbName, config.HomeDir, bstype)
	defer bsdb.Close()

	app := peptide.NewWithOptions(peptide.PeptideAppOptions{
		ChainID:             config.ChainId,
		DB:                  appdb,
		IAVLDisableFastNode: config.IavlDisableFastNode,
		IAVLLazyLoading:     config.IavlLazyLoading,
	}, config.Logger)

	genesis, err := saveGenesis(config, app, bsdb)
	if err != nil {
		return err
	}

	node, err := node.NewPeptideNodeFromConfig(app, bsdb, txIndexerDb, genesis, config)
	if err != nil {
		return err
	}

	if err := node.Service().Start(); err != nil {
		return err
	}

	opnode, err := newOpNodeClient(config)
	if err != nil {
		return err
	}
	logger := server.DefaultLogger()

	logger.Info("producing blocks", "blocks", config.BlocksInStore)
	startBlocks := time.Now()
	opnode.ProduceBlocks(int(config.BlocksInStore))
	logger.Info("producing blocks complete", "elapsed", time.Since(startBlocks), "blocks", config.BlocksInStore)

	if err := node.Service().Stop(); err != nil {
		return err
	}

	latestHeight := node.LastBlockHeight()
	rollbackHeight := latestHeight - config.RollbackBlocks
	logger.Info("rollback profiling...",
		"store", config.RollbackStore,
		"rollback_blocks", config.RollbackBlocks,
		"latest_height", latestHeight,
		"rollback_height", rollbackHeight,
	)

	stopCpuProfiling, err := server.StartCpuProfiler(config)
	if err != nil {
		return err
	}
	defer stopCpuProfiling()

	start := time.Now()
	err = rollbackOneStore(node, config.RollbackStore, rollbackHeight, latestHeight)
	elapsed := time.Since(start)
	if err != nil {
		return err
	}

	logger.Info("rollback profiling...",
		"store", config.RollbackStore,
		"rollback_blocks", config.RollbackBlocks,
		"latest_height", latestHeight,
		"rollback_height", rollbackHeight,
		"elapsed", elapsed,
	)
	return nil
}

func profileStoreRollbackCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "store-rollback",
		Short:   "Runs a simple profiling on rolling back a given store.",
		Aliases: []string{"pr"},
		RunE:    runProfileStoreRollback,
	}

	server.AddProfileRollbackCommandFlags(cmd)
	return cmd
}
