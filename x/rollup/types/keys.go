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
	// wrapped Ethers; cannonically bridged from Ethereum
	ETH = "ETH"
	// KeyL1BlockInfo is the key for the L1BlockInfo
	KeyL1BlockInfo = "L1BlockInfo"
)
