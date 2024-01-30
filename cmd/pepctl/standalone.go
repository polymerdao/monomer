package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/cosmos/cosmos-sdk/telemetry"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/polymerdao/monomer/app/node"
	"github.com/polymerdao/monomer/app/node/server"
	"github.com/polymerdao/monomer/app/peptide"
	peptest "github.com/polymerdao/monomer/testutil/peptide"
	"github.com/polymerdao/monomer/testutil/peptide/eeclient"
	"github.com/spf13/cobra"
)

func standaloneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "standalone",
		Short: "Standalone polymer execution engine",
		RunE:  runStandalone,
	}
	server.AddStandaloneCommandFlags(cmd)
	return cmd
}

func saveGenesis(config *server.Config, app *peptide.PeptideApp, bsdb tmdb.DB) (*node.PeptideGenesis, error) {
	accounts := peptest.Accounts
	validatorAccounts := peptest.ValidatorAccounts
	stateBytes, err := json.Marshal(app.SimpleGenesis(accounts, validatorAccounts))
	if err != nil {
		return nil, err
	}
	genesis := &node.PeptideGenesis{
		GenesisTime: time.Now(),
		AppState:    stateBytes,
		ChainID:     config.ChainId,
	}
	block, err := node.InitChain(app, bsdb, genesis)
	if err != nil {
		return nil, err
	}

	genesis.GenesisBlock = eth.BlockID{
		Hash:   block.Hash(),
		Number: uint64(block.Height()),
	}
	// we need a value so the genesis validation passes.
	genesis.L1 = genesis.GenesisBlock

	if err := genesis.Save(config.HomeDir, config.Override); err != nil {
		return nil, err
	}
	return genesis, nil
}

func newOpNodeClient(config *server.Config) (*peptest.OpNodeMock, error) {
	client, err := eeclient.NewEeClient(
		context.Background(),
		config.PeptideCometServerRpc.FullAddress("http"),
		config.PeptideEngineServerRpc.FullAddress("http"),
	)
	if err != nil {
		return nil, err
	}

	return peptest.NewOpNodeMock(
		nil,
		client,
		rand.New(rand.NewSource(time.Now().UnixNano())),
	), nil
}

func runStandalone(cmd *cobra.Command, args []string) error {
	logger := server.DefaultLogger()
	config := server.NewConfig(cmd).
		WithPeptideCometServerRpc().
		WithPeptideEngineServerRpc().
		WithBlockTime().
		WithDbBackend().
		WithHomeDir().
		WithChainId().
		WithOverride().
		WithPrometheusRetentionTime().
		WithCpuProfile().
		WithPprofRpc().
		WithIavlLazyLoading().
		WithIavlDisableFastNode().
		WithLogger(logger)

	dataDir := filepath.Join(config.HomeDir, "data")
	if config.Override {
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("failed to remove data directory: %w", err)
		}
		if err := os.RemoveAll(filepath.Join(config.HomeDir, "config")); err != nil {
			return fmt.Errorf("failed to remove config directory: %w", err)
		}
	}
	var initGenesis bool
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		initGenesis = true
	}

	appdb := server.OpenDB(node.AppStateDbName, config)
	defer appdb.Close()

	txIndexerDb := server.OpenDB(node.TxStoreDbName, config)
	defer txIndexerDb.Close()

	bsdb := server.OpenDB(node.BlockStoreDbName, config)
	defer bsdb.Close()

	_, err := telemetry.New(
		telemetry.Config{Enabled: true, EnableHostname: false, EnableHostnameLabel: false,
			PrometheusRetentionTime: config.PrometheusRetentionTime},
	)
	if err != nil {
		return err
	}

	app := peptide.NewWithOptions(peptide.PeptideAppOptions{
		ChainID:             config.ChainId,
		DB:                  appdb,
		IAVLDisableFastNode: config.IavlDisableFastNode,
		IAVLLazyLoading:     config.IavlLazyLoading,
	}, logger)

	var genesis *node.PeptideGenesis
	if initGenesis {
		genesis, err = saveGenesis(config, app, bsdb)
	} else {
		genesis, err = node.PeptideGenesisFromFile(config.HomeDir)
	}
	if err != nil {
		return err
	}

	node, err := node.NewPeptideNodeFromConfig(app, bsdb, txIndexerDb, genesis, config)
	if err != nil {
		return err
	}

	stopCpuProfiling, err := server.StartCpuProfiler(config)
	if err != nil {
		return err
	}
	defer stopCpuProfiling()

	if err := node.Service().Start(); err != nil {
		return err
	}

	opnode, err := newOpNodeClient(config)
	if err != nil {
		return err
	}

	// Listen for the kill signals, this tells the client to stop
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGTERM)
	var wg sync.WaitGroup
	ticker := time.NewTicker(config.BlockTime)

	wg.Add(1)
	go func() {
		for {
			select {
			case sig := <-sigCh:
				logger.Info("Received signal", "signal", sig)
				defer wg.Done()
				opnode.Close()
				return
			case <-ticker.C:
				opnode.ProduceBlocks(1)
			default:
			}
		}
	}()
	// Wait for the client to stop, then stop the server
	wg.Wait()
	node.Service().Stop()
	return nil
}
