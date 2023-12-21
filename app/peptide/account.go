package peptide

import (
	"encoding/json"
	"fmt"

	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

var (
	_ AccountI       = (*authtypes.BaseAccount)(nil)
	_ GenesisAccount = (*authtypes.BaseAccount)(nil)
	_ PrivKey        = (*secp256k1.PrivKey)(nil)
)

type GenesisAccount = authtypes.GenesisAccount
type AccountI = authtypes.AccountI
type PrivKey = cryptotypes.PrivKey

// type PrivKey = cryptotypes.LedgerPrivKey

type SignerAccount struct {
	GenesisAccount
	PrivKey
}

func NewSignerAccount(genesisAccount GenesisAccount, privKey PrivKey) *SignerAccount {
	return &SignerAccount{
		GenesisAccount: genesisAccount,
		PrivKey:        privKey,
	}
}

// var _ SignerAccount = (*signerAccount)(nil)

type SignerAccounts []*SignerAccount

func (a SignerAccounts) GetGenesisAccounts() []GenesisAccount {
	accounts := make([]GenesisAccount, len(a))
	for i, acc := range a {
		accounts[i] = acc.GenesisAccount
	}
	return accounts
}

// Addresses return a slice of bech32 encoded addresses prefixed with `Bech32PrefixAccAddr`
func (a SignerAccounts) Addresses() []string {
	addrs := make([]string, len(a))
	for i, acc := range a {
		addrs[i] = acc.GetAddress().String()
	}
	return addrs
}

func (a SignerAccounts) NewBalances(denom string, amount sdk.Int) []banktypes.Balance {
	balances := make([]banktypes.Balance, len(a))
	for i, acc := range a {
		var addrStr string = acc.GetAddress().String()
		balances[i] = banktypes.Balance{
			Address: addrStr,
			Coins:   sdk.NewCoins(sdk.NewCoin(denom, amount)),
		}
	}
	return balances
}

func MergeBalances(balancesSlice ...[]banktypes.Balance) []banktypes.Balance {
	var mergedMap = make(map[string]sdk.Coins)
	for _, balances := range balancesSlice {
		for _, balance := range balances {
			match, ok := mergedMap[balance.Address]
			if ok {
				mergedMap[balance.Address] = match.Add(balance.Coins...)
			} else {
				mergedMap[balance.Address] = balance.Coins
			}
		}
	}
	var merged []banktypes.Balance
	for addr, coins := range mergedMap {
		merged = append(merged, banktypes.Balance{Address: addr, Coins: coins})
	}
	return merged
}

// NewSignerAccounts creates a list of Secp256k1 SignerAccount with the given number of accounts `n` and starting sequence number `startingSeqNum`
func NewSignerAccounts(n uint64, startingSeqNum uint64) SignerAccounts {
	accounts := make([]*SignerAccount, n)
	for i := uint64(0); i < uint64(n); i++ {
		accounts[i] = NewSecp256k1SignerAccount(i, startingSeqNum)
	}
	return accounts
}

// NewSignerAccounts creates a list of ed25519 SignerAccount with the given number of accounts `n` and starting sequence number `startingSeqNum`
func NewValidatorSignerAccounts(n uint64, startingSeqNum uint64) SignerAccounts {
	accounts := make([]*SignerAccount, n)
	for i := uint64(0); i < uint64(n); i++ {
		accounts[i] = NewEd25519SignerAccount(i, startingSeqNum)
	}
	return accounts
}

// NewSecp256k1SignerAccount creates a new SignerAccount with the given account number and sequence number
func NewSecp256k1SignerAccount(acctNumber, seqNumber uint64) *SignerAccount {
	privateKey := secp256k1.GenPrivKey()
	publicKey := privateKey.PubKey()
	acc := authtypes.NewBaseAccount(publicKey.Address().Bytes(), publicKey, acctNumber, seqNumber)
	return NewSignerAccount(acc, privateKey)
}

// NewEd25519SignerAccount creates a new SignerAccount with the given account number and sequence number baced by ed25519
func NewEd25519SignerAccount(acctNumber, seqNumber uint64) *SignerAccount {
	privateKey := ed25519.GenPrivKey()
	publicKey := privateKey.PubKey()
	acc := authtypes.NewBaseAccount(publicKey.Address().Bytes(), publicKey, acctNumber, seqNumber)
	return NewSignerAccount(acc, privateKey)
}

type signerAccountJson struct {
	Address       string `json:"address"`
	AccountNumber uint64 `json:"account_number"`
	Sequence      uint64 `json:"sequence"`
	PrivateKey    []byte `json:"private_key"` // this is base64 encoded in the JSON
}

func (sa *SignerAccount) MarshalJSON() ([]byte, error) {
	// create a signerAccountJson instance
	saj := signerAccountJson{
		Address:       sa.GetAddress().String(),
		AccountNumber: sa.GetAccountNumber(),
		Sequence:      sa.GetSequence(),
		PrivateKey:    sa.PrivKey.Bytes(),
	}

	// marshal the signerAccountJson instance
	bz, err := json.Marshal(saj)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal signer account to JSON: %w", err)
	}

	return bz, nil
}

func (sa *SignerAccount) UnmarshalJSON(bz []byte) error {
	// create a signerAccountJson instance
	var saj signerAccountJson
	err := json.Unmarshal(bz, &saj)
	if err != nil {
		return fmt.Errorf("failed to unmarshal signer account from JSON: %w", err)
	}

	// create a SignerAccount instance
	privKey := &secp256k1.PrivKey{Key: saj.PrivateKey}
	if err != nil {
		return fmt.Errorf("failed to parse private key from JSON: %w", err)
	}
	// create a BaseAccount instance
	addr, err := sdk.AccAddressFromBech32(saj.Address)
	if err != nil {
		return fmt.Errorf("failed to parse address from JSON: %w", err)
	}
	baseAcc := authtypes.NewBaseAccount(addr, privKey.PubKey(), saj.AccountNumber, saj.Sequence)

	sa.GenesisAccount = baseAcc
	sa.PrivKey = privKey

	return nil
}
