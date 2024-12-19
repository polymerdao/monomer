package types

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	const (
		defaultL1FeeRecipient string = "0x000000000000000000000000000000000000dEaD"
		// defaultL1CrossDomainMessenger uses the devnet address of the L1 cross domain messenger contract as the default value.
		defaultL1CrossDomainMessenger string = "0x3d609De69E066F85C38AC274e3EeC251EcfDeAa1"
		// defaultL1StandardBridge uses the devnet address of the L1 standard bridge contract as the default value.
		defaultL1StandardBridge       string = "0x9D34A2610Ea283f6d9AE29f9Cad82e00c4d38507"
		defaultMinFeeWithdrawalAmount uint64 = 400_000
		defaultFeeWithdrawalGasLimit  uint64 = 400_000
	)

	return Params{
		L1FeeRecipient:         defaultL1FeeRecipient,
		L1CrossDomainMessenger: defaultL1CrossDomainMessenger,
		L1StandardBridge:       defaultL1StandardBridge,
		MinFeeWithdrawalAmount: defaultMinFeeWithdrawalAmount,
		FeeWithdrawalGasLimit:  defaultFeeWithdrawalGasLimit,
	}
}

// Validate checks that the parameters have valid values.
func (p *Params) Validate() error {
	if err := validateEthAddress(p.L1FeeRecipient); err != nil {
		return fmt.Errorf("validate L1 fee recipient address: %w", err)
	}
	if err := validateEthAddress(p.L1CrossDomainMessenger); err != nil {
		return fmt.Errorf("validate L1 cross domain messenger address: %w", err)
	}
	if err := validateEthAddress(p.L1StandardBridge); err != nil {
		return fmt.Errorf("validate L1 standard bridge address: %w", err)
	}

	return nil
}

func validateEthAddress(addr string) error {
	if !common.IsHexAddress(addr) {
		return fmt.Errorf("validate ethereum address: %s", addr)
	}
	return nil
}
