package types

const (
	AttributeKeyL1DepositTxType   = "l1_deposit_tx_type"
	AttributeKeyL2WithdrawalTx    = "l2_withdrawal_tx"
	AttributeKeyL2FeeWithdrawalTx = "l2_fee_withdrawal_tx"
	AttributeKeyFromCosmosAddress = "from_address"
	AttributeKeyToCosmosAddress   = "to_address"
	AttributeKeyMintCosmosAddress = "mint_address"
	AttributeKeySender            = "sender"
	AttributeKeyL1Target          = "target"
	AttributeKeyValue             = "value"
	AttributeKeyMint              = "mint"
	AttributeKeyGasLimit          = "gas_limit"
	AttributeKeyData              = "data"
	AttributeKeyNonce             = "nonce"
	AttributeKeyERC20Address      = "erc20_address"

	L1UserDepositTxType = "l1_user_deposit"

	EventTypeMintETH             = "mint_eth"
	EventTypeMintERC20           = "mint_erc20"
	EventTypeBurnETH             = "burn_eth"
	EventTypeBurnERC20           = "burn_erc20"
	EventTypeWithdrawalInitiated = "withdrawal_initiated"
)
