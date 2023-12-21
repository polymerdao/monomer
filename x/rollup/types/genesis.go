package types

// DefaultGenesis returns a default rollup module genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{}
}

func (m *GenesisState) Validate() error {
	return nil
}
