package types

const (
	AttributeKeyL1DepositTxType   = "l1_deposit_tx_type"
	AttributeKeyL2WithdrawalTx    = "l2_withdrawal_tx"
	AttributeKeyFromCosmosAddress = "from_cosmos_address"
	AttributeKeyToCosmosAddress   = "to_cosmos_address"
	AttributeKeySender            = "sender"
	AttributeKeyL1Target          = "target"
	AttributeKeyValue             = "value"
	AttributeKeyGasLimit          = "gas_limit"
	AttributeKeyData              = "data"
	AttributeKeyNonce             = "nonce"
	AttributeKeyERC20Address      = "erc20_address"

	L1UserDepositTxType = "l1_user_deposit"

	EventTypeMintETH             = "mint_eth"
	EventTypeMintERC20           = "mint_erc20"
	EventTypeBurnETH             = "burn_eth"
	EventTypeWithdrawalInitiated = "withdrawal_initiated"
)
