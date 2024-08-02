package types

const (
	AttributeKeyL1DepositTxType   = "l1_deposit_tx_type"
	AttributeKeyL2WithdrawalTx    = "l2_withdrawal_tx"
	AttributeKeyBridgedTokenType  = "bridged_token_type"
	AttributeKeyFromEvmAddress    = "from_evm_address"
	AttributeKeyToEvmAddress      = "to_evm_address"
	AttributeKeyFromCosmosAddress = "from_cosmos_address"
	AttributeKeyToCosmosAddress   = "to_cosmos_address"
	AttributeKeySender            = "sender"
	AttributeKeyL1Target          = "target"
	AttributeKeyValue             = "value"
	AttributeKeyGasLimit          = "gas_limit"
	AttributeKeyData              = "data"
	AttributeKeyNonce             = "nonce"

	L1SystemDepositTxType = "l1_system_deposit"
	L1UserDepositTxType   = "l1_user_deposit"

	EventTypeMintETH             = "mint_eth"
	EventTypeBurnETH             = "burn_eth"
	EventTypeWithdrawalInitiated = "withdrawal_initiated"
)
