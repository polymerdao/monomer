package genesis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	abci "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/contracts"
)

type Genesis struct {
	Time     uint64                     `json:"time"`
	ChainID  monomer.ChainID            `json:"chain_id"`
	AppState map[string]json.RawMessage `json:"app_state"`
}

type DB interface {
	AppendBlock(*monomer.Block) error
	UpdateLabels(unsafe, safe, finalized common.Hash) error
}

const defaultGasLimit = 30_000_000

// Commit assumes the application has not been initialized and that the block store is empty.
func (g *Genesis) Commit(ctx context.Context, app monomer.Application, blockStore DB, ethstatedb state.Database) error {
	appStateBytes, err := json.Marshal(g.AppState)
	if err != nil {
		return fmt.Errorf("marshal app state: %v", err)
	}

	const initialHeight = 1
	if _, err = app.InitChain(ctx, &abci.RequestInitChain{
		ChainId:       g.ChainID.String(),
		AppStateBytes: appStateBytes,
		Time:          time.Unix(int64(g.Time), 0),
		// If the initial height is not set, the cosmos-sdk will silently set it to 1.
		// https://github.com/cosmos/cosmos-sdk/issues/19765
		InitialHeight: initialHeight,
	}); err != nil {
		return fmt.Errorf("init chain: %v", err)
	}

	header := &monomer.Header{
		Height:   initialHeight,
		ChainID:  g.ChainID,
		Time:     g.Time,
		GasLimit: defaultGasLimit,
	}
	cometHeader := header.ToComet()
	info, err := app.Info(ctx, &abci.RequestInfo{})
	if err != nil {
		return fmt.Errorf("info: %v", err)
	}
	cometHeader.AppHash = info.GetLastBlockAppHash()

	if _, err := app.FinalizeBlock(ctx, &abci.RequestFinalizeBlock{
		Hash:               cometHeader.Hash(),
		Height:             cometHeader.Height,
		Time:               cometHeader.Time,
		NextValidatorsHash: cometHeader.NextValidatorsHash,
		ProposerAddress:    cometHeader.ProposerAddress,
	}); err != nil {
		return fmt.Errorf("finalize block: %v", err)
	}

	if _, err := app.Commit(ctx, &abci.RequestCommit{}); err != nil {
		return fmt.Errorf("commit: %v", err)
	}

	// Create ethereum genesis state.
	ethState, err := state.New(gethtypes.EmptyRootHash, ethstatedb, nil)
	if err != nil {
		return fmt.Errorf("create ethereum state: %v", err)
	}
	ethStateRoot, err := contracts.Predeploy(ethState).Commit(initialHeight, true)
	if err != nil {
		return fmt.Errorf("commit ethereum genesis state: %v", err)
	}

	// Create monomer block
	header.StateRoot = ethStateRoot
	block, err := monomer.MakeBlock(header, bfttypes.Txs{})
	if err != nil {
		return fmt.Errorf("make block: %v", err)
	}

	fmt.Println("Genesis block hash:", block.Header.Hash.String())

	if err := blockStore.AppendBlock(block); err != nil {
		return fmt.Errorf("append block: %v", err)
	}
	if err := blockStore.UpdateLabels(block.Header.Hash, block.Header.Hash, block.Header.Hash); err != nil {
		panic(fmt.Errorf("update labels: %v", err)) // TODO database is corrupted if this gets hit. Big problems.
	}
	return nil
}
