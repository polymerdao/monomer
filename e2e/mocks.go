package e2e

import (
	"fmt"

	"github.com/cosmos/cosmos-sdk/client"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

type MockAccountRetriever struct {
	Accounts                   map[string]*authtypes.BaseAccount
	ReturnAccNum, ReturnAccSeq uint64
}

func (mar MockAccountRetriever) GetAccount(ctx client.Context, addr sdktypes.AccAddress) (client.Account, error) {
	acc, ok := mar.Accounts[addr.String()]
	if !ok {
		return nil, fmt.Errorf("account not found")
	}
	return acc, nil
}

func (mar MockAccountRetriever) GetAccountWithHeight(
	_ client.Context, //nolint:gocritic // hugeParam
	_ sdktypes.AccAddress,
) (client.Account, int64, error) {
	return mockAccount{}, 0, nil
}

func (mar MockAccountRetriever) EnsureExists(_ client.Context, _ sdktypes.AccAddress) error { //nolint:gocritic // hugeParam
	return nil
}

func (mar MockAccountRetriever) GetAccountNumberSequence(
	_ client.Context, //nolint:gocritic // hugeParam
	_ sdktypes.AccAddress,
) (uint64, uint64, error) {
	return mar.ReturnAccNum, mar.ReturnAccSeq, nil
}

type mockAccount struct {
	address   sdktypes.AccAddress
	seqNumber uint64
}

func (ma mockAccount) GetAddress() sdktypes.AccAddress {
	return ma.address
}

func (ma mockAccount) GetPubKey() cryptotypes.PubKey {
	return nil
}

func (ma mockAccount) GetAccountNumber() uint64 {
	return 1
}

func (ma mockAccount) GetSequence() uint64 {
	ma.seqNumber++
	return ma.seqNumber
}
