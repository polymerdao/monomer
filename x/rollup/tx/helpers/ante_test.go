package helpers_test

import (
	"testing"

	storetypes "cosmossdk.io/store/types"
	"cosmossdk.io/x/tx/signing"
	"cosmossdk.io/x/tx/signing/direct"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	authante "github.com/cosmos/cosmos-sdk/x/auth/ante"
	authantetestutil "github.com/cosmos/cosmos-sdk/x/auth/ante/testutil"
	authtestutil "github.com/cosmos/cosmos-sdk/x/auth/testutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/golang/mock/gomock"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/polymerdao/monomer/testutils"
	"github.com/polymerdao/monomer/x/rollup/tx/helpers"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type mockRollupKeeper struct{}

func (mockRollupKeeper) GetL1BlockInfo(_ sdk.Context) (*rolluptypes.L1BlockInfo, error) { //nolint:gocritic // hugeParam
	return &rolluptypes.L1BlockInfo{}, nil
}

type mockTx struct {
	tx *sdktx.Tx
}

var _ sdk.Tx = (*mockTx)(nil)

func (tx *mockTx) GetMsgs() []sdk.Msg {
	return tx.tx.GetMsgs()
}

func (tx *mockTx) GetMsgsV2() ([]protoreflect.ProtoMessage, error) {
	msgsV1 := tx.GetMsgs()
	msgs := make([]protoreflect.ProtoMessage, 0, len(msgsV1))
	for _, msgV1 := range msgsV1 {
		msgs = append(msgs, protov1.MessageV2(msgV1))
	}
	return msgs, nil
}

func TestUserTransactionCannotContainDepositMesssage(t *testing.T) {
	sdkCtx := testutil.DefaultContextWithDB(
		t,
		storetypes.NewKVStoreKey(rolluptypes.StoreKey),
		storetypes.NewTransientStoreKey("transient_test"),
	).Ctx
	anteHandler, err := helpers.NewAnteHandler(authante.HandlerOptions{
		AccountKeeper:   authantetestutil.NewMockAccountKeeper(gomock.NewController(t)),
		BankKeeper:      authtestutil.NewMockBankKeeper(gomock.NewController(t)),
		SignModeHandler: signing.NewHandlerMap(direct.SignModeHandler{}),
	}, mockRollupKeeper{})
	require.NoError(t, err)

	privKey, err := crypto.GenerateKey()
	require.NoError(t, err)
	for _, depositMsg := range []protov1.Message{&rolluptypes.MsgSetL1Attributes{}, &rolluptypes.MsgApplyUserDeposit{}} {
		_, err := anteHandler.AnteHandle(sdkCtx, &mockTx{
			tx: testutils.BuildSDKTx(t, "chainID", 0, 0, privKey, []protov1.Message{depositMsg}),
		}, false)
		require.ErrorContains(t, err, "transaction contains deposit message")
	}
}
