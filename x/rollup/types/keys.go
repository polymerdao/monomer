package types

import "github.com/cosmos/cosmos-sdk/types"

const (
	// ModuleName defines the module name
	ModuleName = "rollup"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// QuerierRoute defines the module's query routing key
	QuerierRoute = ModuleName

	// MemStoreKey defines the in-memory store key
	MemStoreKey = "mem_rollup"
)

const (
	// WEI is the denom for wrapped Ether (represented in wei) canonically bridged from Ethereum
	WEI = "wei"
	// KeyL1BlockInfo is the key for the L1BlockInfo
	KeyL1BlockInfo   = "L1BlockInfo"
	KeyCollectedFees = "CollectedFees"
)

var L1FeeVaultAddress = types.AccAddress("l1_fee_collector")
