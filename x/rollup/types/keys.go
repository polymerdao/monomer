package types

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
	// L1BlockInfoKey is the store key for the L1BlockInfo
	L1BlockInfoKey = "l1_block_info"
	// ParamsKey is the store key for the x/rollup module parameters
	ParamsKey = "params"
)
