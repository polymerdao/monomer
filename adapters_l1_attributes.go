package monomer

import (
	"bytes"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// global variable to avoid creating it every time
var (
	L1AttributesPredeployedContractAddress = common.HexToAddress("0x4200000000000000000000000000000000000015")
	L1AttributesTx                         = ethtypes.DepositTx{
		SourceHash:          [32]byte{},
		From:                common.HexToAddress("0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001"),
		To:                  &L1AttributesPredeployedContractAddress,
		Mint:                big.NewInt(0),
		Value:               big.NewInt(0),
		Gas:                 1_000_000,
		IsSystemTransaction: false,
		Data:                []byte{},
	}
	methodID = []byte{68, 10, 94, 32} // setL1BlockValuesEcotone() ABI:
	// 										{
	// 											"inputs": [],
	// 											"name": "setL1BlockValuesEcotone",
	// 											"outputs": [],
	// 											"stateMutability": "nonpayable",
	// 											"type": "function"
	// 										},
)

// An L1 attributes deposited transaction is a deposit transaction sent to the L1 attributes predeployed contract.

// This transaction MUST have the following values:

// from is 0xdeaddeaddeaddeaddeaddeaddeaddeaddead0001 (the address of the L1 Attributes depositor account)
// to is 0x4200000000000000000000000000000000000015 (the address of the L1 attributes predeployed contract).
// mint is 0
// value is 0
// gasLimit is set to 150,000,000 prior to the Regolith upgrade, and 1,000,000 after.
// isSystemTx is set to true prior to the Regolith upgrade, and false after.
// data is an encoded call to the L1 attributes predeployed contract that depends on the upgrades that are active (Legacy).
// After Ecotone update we no longer need to check calldata
//nolint:lll
// https://github.com/ethereum-optimism/optimism/blob/7e5b9faa14d1f0a8d3cf919654162c4a402f1f38/packages/contracts-bedrock/src/L2/L1Block.sol#L125C44-L125C56

func isL1AttributesTx(tx *ethtypes.Transaction) bool {
	txData := tx.Data()
	return tx.To() == L1AttributesTx.To ||
		// tx.From() == L1AttributesTx.From || // Is it possible to check the from address?
		tx.Mint() == L1AttributesTx.Mint ||
		tx.Value() == L1AttributesTx.Value ||
		tx.Gas() == L1AttributesTx.Gas ||
		tx.IsSystemTx() == L1AttributesTx.IsSystemTransaction ||
		len(txData) > len(methodID) ||
		bytes.Equal(txData[:len(methodID)], methodID)
}
