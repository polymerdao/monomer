package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/polymerdao/monomer/x/rollup/types"
)

// InitGenesis - Init store state from genesis data
func (k *Keeper) InitGenesis(ctx sdk.Context, state types.GenesisState) { //nolint:gocritic // hugeParam
	if err := k.SetParams(ctx, &state.Params); err != nil {
		panic(fmt.Errorf("failed to set params: %w", err))
	}
}

// ExportGenesis returns a GenesisState for a given context and keeper
func (k *Keeper) ExportGenesis(ctx sdk.Context) *types.GenesisState { //nolint:gocritic // hugeParam
	params, err := k.GetParams(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to get params: %w", err))
	}

	return types.NewGenesisState(*params)
}
