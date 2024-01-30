package peptest

import (
	"math/rand"
	"time"

	tmdb "github.com/cometbft/cometbft-db"
	ctypes "github.com/cometbft/cometbft/rpc/core/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/polymerdao/monomer/app/node/server"
	eetypes "github.com/polymerdao/monomer/app/node/types"
	"github.com/polymerdao/monomer/app/peptide"
	"github.com/polymerdao/monomer/testutil/peptide/eeclient"
	"github.com/polymerdao/monomer/x/rollup/types"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

type OpNodeMock struct {
	*suite.Suite
	*eeclient.EeClient

	rng       *rand.Rand
	info      eth.BlockInfo
	sysConfig eth.SystemConfig
	rollupCfg *rollup.Config

	// use a dummy app here for signing txs on the client side
	app *peptide.PeptideApp
}

func NewOpNodeMock(suite *suite.Suite, client *eeclient.EeClient, rng *rand.Rand) *OpNodeMock {
	logger := server.DefaultLogger()
	info := testutils.RandomBlockInfo(rng)
	// set an arbitrary fixed block number for tests
	info.InfoNum = 42
	sysConfig := eth.SystemConfig{
		BatcherAddr: testutils.RandomAddress(rng),
		Overhead:    [32]byte{},
		Scalar:      [32]byte{},
		GasLimit:    1234567,
	}

	chainID, err := client.ChainID()
	if err != nil {
		panic(err)
	}
	rollup := &rollup.Config{}
	return &OpNodeMock{
		Suite:     suite,
		EeClient:  client,
		rng:       rng,
		info:      info,
		sysConfig: sysConfig,
		rollupCfg: rollup,
		app:       peptide.New(chainID.String(), "", tmdb.NewMemDB(), logger),
	}
}

func (o *OpNodeMock) ProduceBlocks(numBlocks int) {
	for i := 0; i < numBlocks; i++ {
		payloadId := o.StartPayload(uint64(i), nil)
		blockHash := o.ConfirmPayload(payloadId)
		o.UpdateL2Head(blockHash, i%2 != 0)
	}
}

func (o *OpNodeMock) RunDerivationLoop(numBlocks uint64, sender, receiver *peptide.SignerAccount) {
	lastBlock, err := o.BlockByNumber(eth.Unsafe)
	o.NoError(err)

	// fork-only mode doesn't change anything, but should succeed
	fcResult, err := o.ForkchoiceUpdated(
		&eth.ForkchoiceState{HeadBlockHash: lastBlock.Hash()},
		&eth.PayloadAttributes{},
	)
	o.NoError(err)
	o.Equal(eth.ExecutionValid, fcResult.PayloadStatus.Status)
	o.Nil(fcResult.PayloadID)

	// create a new payload
	// new-payload mode starts a new payload
	// and returns a ForkchoiceUpdateResult with the new payload'o ID

	lastBlockHash := lastBlock.Hash()

	// safe and finalized block default to genesis blockHash
	safe, err := o.BlockByNumber(eth.Safe)
	o.NoError(err)
	finalized, err := o.BlockByNumber(eth.Finalized)
	o.NoError(err)

	o.Equal(lastBlockHash, safe.Hash())
	o.Equal(lastBlockHash, finalized.Hash())

	for i := uint64(1); i <= numBlocks; i++ {
		seqNum := i - 1
		// must match the deposit tx in the payload
		l1InfoDepositTx := lo.Must(derive.L1InfoDeposit(o.rollupCfg, o.sysConfig, seqNum, o.info, 0))

		payloadId := o.StartPayload(seqNum, nil)

		// L2 chain gathers L2 txs before block is sealed
		o.DeliverL2Txs(i-1, sender, receiver)

		blockHash := o.ConfirmPayload(payloadId)

		// query blocks by height
		block, err := o.BlockByNumber(i)
		if o.NoError(err) {
			o.Equal(i, block.NumberU64())
			o.Greater(block.Time(), uint64(0))
		}

		// query blocks by hash
		block, err = o.BlockByHash(block.Hash())
		if o.NoError(err) {
			o.Equal(i, block.NumberU64())
			o.Greater(block.Time(), uint64(0))
		}
		// after payload is confirmed, query returns the last committed L1BlockInfo
		request := types.QueryL1BlockInfoRequest{}
		result, err := o.EeClient.ABCIQuery(
			"/rollup.Query/L1BlockInfo",
			lo.Must(request.Marshal()),
			0,
			false,
		)
		o.NoError(err)
		var response types.QueryL1BlockInfoResponse
		lo.Must0(response.Unmarshal(result.Response.Value))
		l1BlockInfoFromQuery := types.FromL1BlockInfoResponse(&response)
		// l1BlockInfo is binary encoded
		o.Equal(l1InfoDepositTx.Data, lo.Must(derive.L1BlockInfoToBytes(o.rollupCfg, 0, l1BlockInfoFromQuery)))

		// block head shouldn't have changed yet
		// processed payloads are not immediately canonical
		unsafe, err := o.BlockByNumber(eth.Unsafe)
		o.NoError(err)
		o.Equal(lastBlockHash, unsafe.Hash())

		o.UpdateL2Head(blockHash, (i-1)%2 != 0)

		currentBlock, err := o.PeptideBlock(eth.Unsafe)
		o.NoError(err)
		lastBlockHash = currentBlock.Hash()

		// confirm L2 txs are included
		o.EqualValues(i-1, currentBlock.Txs.Len())

		for _, tx := range currentBlock.Txs {
			// can query the tx by hash
			txHash := tx.Hash()
			txFound, err := o.Tx(txHash, false)
			o.NoError(err)
			// some txs may fail due to invalid pubKey or account seq num, which is ok
			o.Equal(txFound.Height, currentBlock.Height())
		}
	}

	// finalized block hash is still the genesis block
	{
		block, err := o.BlockByNumber("1")
		o.NoError(err)
		finalized, err := o.BlockByNumber(eth.Finalized)
		o.NoError(err)
		o.Equal(block.Hash(), finalized.Hash())
	}

	// update finalized head
	{
		finalized, err := o.BlockByNumber(eth.Finalized)
		o.NoError(err)
		headBlock, err := o.BlockByNumber(eth.Unsafe)
		o.NoError(err)
		fcs := eth.ForkchoiceState{
			HeadBlockHash:      headBlock.Hash(),
			FinalizedBlockHash: finalized.Hash(),
		}
		fcResult, err = o.ForkchoiceUpdated(&fcs, &eth.PayloadAttributes{})
		o.NoError(err)
		o.Equal(eth.ExecutionValid, fcResult.PayloadStatus.Status)
		last, err := o.BlockByNumber(eth.Finalized)
		o.NoError(err)
		o.Equal(finalized.Hash(), last.Hash())
	}
}

func (o *OpNodeMock) StartPayload(
	seqNum uint64,
	headBlockHash *eetypes.Hash,
	userDepositTxs ...hexutil.Bytes,
) *eth.PayloadID {
	l1InfoTxBytes := lo.Must(derive.L1InfoDepositBytes(o.rollupCfg, o.sysConfig, seqNum, o.info, 0))

	txs := []hexutil.Bytes{l1InfoTxBytes}
	txs = append(txs, userDepositTxs...)

	if headBlockHash == nil {
		unsafe, err := o.BlockByNumber(eth.Unsafe)
		o.NoError(err)
		hash := unsafe.Hash()
		headBlockHash = &hash
	}

	gasLimit := eth.Uint64Quantity(o.sysConfig.GasLimit)
	fcResult, err := o.ForkchoiceUpdated(
		&eth.ForkchoiceState{HeadBlockHash: *headBlockHash},
		&eth.PayloadAttributes{
			Timestamp:    hexutil.Uint64(time.Unix(1699646689, int64(seqNum)).Unix()),
			Transactions: txs,
			GasLimit:     &gasLimit,
		},
	)
	o.NoError(err)
	o.Equal(eth.ExecutionValid, fcResult.PayloadStatus.Status)
	o.NotNil(fcResult.PayloadID)

	return fcResult.PayloadID
}

func (o *OpNodeMock) ConfirmPayload(payloadId *eth.PayloadID) eetypes.Hash {
	payload, err := o.GetPayload(payloadId)
	o.NoError(err)
	o.NotNil(payload)

	payloadStatus, err := o.NewPayload(payload.ExecutionPayload)
	o.NoError(err)
	o.Equal(eth.ExecutionValid, payloadStatus.Status)
	return payload.ExecutionPayload.BlockHash
}

func (o *OpNodeMock) UpdateL2Head(headBlockHash eetypes.Hash, updateSafe bool) {
	// fork-only mode updates the canonical block pointer
	fcs := eth.ForkchoiceState{HeadBlockHash: headBlockHash}

	if updateSafe {
		fcs.SafeBlockHash = headBlockHash
	}
	fcResult, err := o.ForkchoiceUpdated(&fcs, &eth.PayloadAttributes{})
	o.NoError(err)
	o.Equal(eth.ExecutionValid, fcResult.PayloadStatus.Status)
	block, err := o.BlockByNumber(eth.Unsafe)
	o.NoError(err)
	o.Equal(headBlockHash, block.Hash())

	if updateSafe {
		safeBlockRef, err := o.BlockByNumber(eth.Safe)
		o.NoError(err)
		o.Equal(headBlockHash, safeBlockRef.Hash())
	}
}

func (o *OpNodeMock) SendTokens(
	sender, receiver *peptide.SignerAccount,
	amount int64,
) (*ctypes.ResultBroadcastTx, error) {
	msg := banktypes.NewMsgSend(
		sender.GetAddress(),
		receiver.GetAddress(),
		sdk.NewCoins(sdk.NewInt64Coin(o.app.BondDenom, amount)),
	)
	return o.SendMsg(sender, msg)
}

func (o *OpNodeMock) DeliverL2Txs(numTxs uint64, sender, receiver *peptide.SignerAccount) {
	for i := uint64(0); i < numTxs; i++ {
		o.SendTokens(sender, receiver, int64(o.rng.Intn(100)+1))
		// TODO: increment sequence number; but doing so would fail genesis reproducibility tests
	}
}

func (o *OpNodeMock) GetAccountSequenceNumber(account *peptide.SignerAccount) (uint64, error) {
	query := &authtypes.QueryAccountRequest{
		Address: account.GetAddress().String(),
	}
	data, err := query.Marshal()
	if err != nil {
		return 0, err
	}
	result, err := o.ABCIQuery("/cosmos.auth.v1beta1.Query/Account", data, 0, false)
	if err != nil {
		return 0, err
	}
	var response authtypes.QueryAccountResponse
	if err := response.Unmarshal(result.Response.Value); err != nil {
		return 0, err
	}
	var accountI authtypes.AccountI
	if err := o.app.InterfaceRegistry().UnpackAny(response.Account, &accountI); err != nil {
		return 0, err
	}
	return accountI.GetSequence(), nil
}

func (o *OpNodeMock) SendMsg(sender *peptide.SignerAccount, msg sdk.Msg) (*ctypes.ResultBroadcastTx, error) {
	sequence, err := o.GetAccountSequenceNumber(sender)
	o.NoError(err)
	sender.SetSequence(sequence)

	// Sign the transaction
	tx, err := peptide.GenTx(
		o.app.EncodingConfig.TxConfig,
		[]sdk.Msg{msg},
		sdk.Coins{sdk.NewInt64Coin(o.app.BondDenom, 0)},
		peptide.DefaultGenTxGas,
		o.app.ChainId,
		o.rng,
		sender,
	)
	o.NoErrorf(err, "failed to sign tx by account %o", sender.GetAddress())

	// encode the transaction
	txBytes, err := o.app.EncodingConfig.TxConfig.TxEncoder()(tx)
	o.NoError(err, "failed to encode tx")

	receipt, err := o.BroadcastTxSync(txBytes)
	if err != nil {
		return nil, err
	}

	sender.SetSequence(sender.GetSequence() + 1)
	return receipt, nil
}

func (o *OpNodeMock) QueryBalance(address string, height int64) *sdktypes.Coin {
	request := &banktypes.QueryBalanceRequest{Address: address, Denom: sdktypes.DefaultBondDenom}
	data, err := request.Marshal()
	o.NoError(err)
	result, err := o.ABCIQuery(
		"/cosmos.bank.v1beta1.Query/Balance",
		data,
		height,
		false,
	)
	o.NoError(err)
	o.Equal(uint32(0), result.Response.Code)

	var response banktypes.QueryBalanceResponse
	o.NoError(response.Unmarshal(result.Response.Value))

	return response.Balance
}

func (o *OpNodeMock) CurrentBlock() *ethtypes.Block {
	block, err := o.BlockByNumber(eth.Unsafe)
	o.NoError(err)
	return block
}

// These override the suite assertions in case the mock is used outside of a test. In that case, the Suite attribute
// will be nil and we don't perform the required checks.
func (o *OpNodeMock) NoError(err error, msgAndArgs ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.NoError(err, msgAndArgs...)
	}
	return true
}

func (o *OpNodeMock) NoErrorf(err error, msg string, args ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.NoErrorf(err, msg, args...)
	}
	return true
}

func (o *OpNodeMock) Greater(e1 interface{}, e2 interface{}, msgAndArgs ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.Greater(e1, e2, msgAndArgs...)
	}
	return true
}

func (o *OpNodeMock) Equal(expected interface{}, actual interface{}, msgAndArgs ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.Equal(expected, actual, msgAndArgs...)
	}
	return true
}

func (o *OpNodeMock) Nil(object interface{}, msgAndArgs ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.Nil(object, msgAndArgs...)
	}
	return true
}

func (o *OpNodeMock) NotNil(object interface{}, msgAndArgs ...interface{}) bool {
	if o.Suite != nil {
		return o.Suite.NotNil(object, msgAndArgs...)
	}
	return true
}
