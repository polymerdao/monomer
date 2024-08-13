package e2e

import (
	"github.com/cosmos/cosmos-sdk/client"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
)

type mockAccountRetriever struct {
	ReturnAccNum, ReturnAccSeq uint64
}

func (mar mockAccountRetriever) GetAccount(_ client.Context, _ sdktypes.AccAddress) (client.Account, error) { //nolint:gocritic // hugeParam
	return mockAccount{}, nil
}

func (mar mockAccountRetriever) GetAccountWithHeight(
	_ client.Context, //nolint:gocritic // hugeParam
	_ sdktypes.AccAddress,
) (client.Account, int64, error) {
	return mockAccount{}, 0, nil
}

func (mar mockAccountRetriever) EnsureExists(_ client.Context, _ sdktypes.AccAddress) error { //nolint:gocritic // hugeParam
	return nil
}

func (mar mockAccountRetriever) GetAccountNumberSequence(
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
