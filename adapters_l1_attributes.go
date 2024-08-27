package monomer

import (
	"math/big"

	"github.com/ethereum-optimism/optimism/op-node/chaincfg"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// L1 Attributes Deposited Transaction
// https://specs.optimism.io/protocol/deposits.html#l1-attributes-deposited-transaction
// After Ecotone update we no longer need to check calldata
//nolint:lll
// https://github.com/ethereum-optimism/optimism/blob/7e5b9faa14d1f0a8d3cf919654162c4a402f1f38/packages/contracts-bedrock/src/L2/L1Block.sol#L125C44-L125C56

func IsL1AttributesTx(tx *ethtypes.Transaction) bool {
	if mint := tx.Mint(); mint != nil && mint.Cmp(big.NewInt(0)) != 0 {
		return false
	}

	rollupCfg := chaincfg.Mainnet // TODO: Can we get this?
	timestamp := uint64(tx.Time().Unix())

	_, err := derive.L1BlockInfoFromBytes(rollupCfg, timestamp, tx.Data())
	if err != nil {
		return false
	}

	isSystemTransaction := true
	gas := uint64(150_000_000) //nolint:mnd
	if rollupCfg.IsRegolith(timestamp) {
		isSystemTransaction = false
		gas = derive.RegolithSystemTxGas
	}

	return tx.To().Cmp(derive.L1BlockAddress) == 0 &&
		tx.Value().Cmp(big.NewInt(0)) == 0 &&
		tx.Gas() == gas &&
		tx.IsSystemTx() == isSystemTransaction
	// TODO: Can we check From field?
}
