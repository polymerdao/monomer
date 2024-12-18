package testutils

import (
	"crypto/ecdsa"
	"math/big"
	"math/rand"
	"strings"
	"testing"

	txv1beta1 "cosmossdk.io/api/cosmos/tx/v1beta1"
	"cosmossdk.io/math"
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/vfs"
	cometdb "github.com/cometbft/cometbft-db"
	bfttypes "github.com/cometbft/cometbft/types"
	dbm "github.com/cosmos/cosmos-db"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cosmossecp256k1 "github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdktypes "github.com/cosmos/cosmos-sdk/types"
	sdktx "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/cosmos-sdk/types/tx/signing"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain"
	opbindings "github.com/ethereum-optimism/optimism/op-chain-ops/crossdomain/bindings"
	"github.com/ethereum-optimism/optimism/op-node/rollup"
	"github.com/ethereum-optimism/optimism/op-node/rollup/derive"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum-optimism/optimism/op-service/testutils"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	gethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
	protov1 "github.com/golang/protobuf/proto" //nolint:staticcheck
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/monomerdb/localdb"
	"github.com/polymerdao/monomer/utils"
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func NewCometMemDB(t *testing.T) cometdb.DB {
	db := cometdb.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func NewMemDB(t *testing.T) dbm.DB {
	db := dbm.NewMemDB()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return db
}

func NewLocalMemDB(t *testing.T) *localdb.DB {
	db, err := pebble.Open("", &pebble.Options{
		FS: vfs.NewMem(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	return localdb.New(db)
}

// GenerateEthTxs generates an L1 attributes tx, deposit tx, and cosmos tx packed in an Ethereum transaction.
// The transactions are not meant to be executed.
func GenerateEthTxs(t *testing.T) (*gethtypes.Transaction, *gethtypes.Transaction, *gethtypes.Transaction) {
	l1Block := GenerateL1Block()
	l1InfoRawTx, err := derive.L1InfoDeposit(&rollup.Config{
		Genesis:     rollup.Genesis{L2: eth.BlockID{Number: 0}},
		L2ChainID:   big.NewInt(1234),
		EcotoneTime: utils.Ptr(uint64(0)),
	}, eth.SystemConfig{}, 0, eth.BlockToInfo(l1Block), l1Block.Time())
	require.NoError(t, err)
	l1InfoTx := gethtypes.NewTx(l1InfoRawTx)

	rng := rand.New(rand.NewSource(1234))
	depositRawTx := testutils.GenerateDeposit(testutils.RandomHash(rng), rng)
	depositRawTx.Mint = big.NewInt(100)
	depositRawTx.Value = big.NewInt(50)
	depositTx := gethtypes.NewTx(depositRawTx)

	cosmosEthTx := monomer.AdaptNonDepositCosmosTxToEthTx([]byte{1})
	return l1InfoTx, depositTx, cosmosEthTx
}

func GenerateEthBridgeDepositTx(t *testing.T, userAddr common.Address, amount *big.Int) *gethtypes.Transaction {
	// We should technically use the ABI for the L2StandardBridge, but we only have the L1 bindings and they work fine here.
	standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.L1StandardBridgeMetaData.ABI))
	require.NoError(t, err)
	rng := rand.New(rand.NewSource(1234))

	finalizeBridgeETHBz, err := standardBridgeABI.Pack(
		"finalizeBridgeETH",
		testutils.RandomAddress(rng), // from
		userAddr,                     // to
		amount,                       // amount
		[]byte{},                     // extra data
	)
	require.NoError(t, err)

	return generateCrossDomainDepositTx(t, finalizeBridgeETHBz)
}

func GenerateERC20DepositTx(t *testing.T, tokenAddr, userAddr common.Address, amount *big.Int) *gethtypes.Transaction {
	// We should technically use the ABI for the L2StandardBridge, but we only have the L1 bindings and they work fine here.
	standardBridgeABI, err := abi.JSON(strings.NewReader(opbindings.L1StandardBridgeMetaData.ABI))
	require.NoError(t, err)
	rng := rand.New(rand.NewSource(1234))

	finalizeBridgeERC20Bz, err := standardBridgeABI.Pack(
		"finalizeBridgeERC20",
		tokenAddr,                    // l1 token address
		testutils.RandomAddress(rng), // l2 token address
		testutils.RandomAddress(rng), // from
		userAddr,                     // to
		amount,                       // amount
		[]byte{},                     // extra data
	)
	require.NoError(t, err)

	return generateCrossDomainDepositTx(t, finalizeBridgeERC20Bz)
}

func generateCrossDomainDepositTx(t *testing.T, crossDomainMessageBz []byte) *gethtypes.Transaction {
	crossDomainMessengerABI, err := abi.JSON(strings.NewReader(opbindings.L2CrossDomainMessengerMetaData.ABI))
	require.NoError(t, err)
	rng := rand.New(rand.NewSource(1234))

	relayMessageBz, err := crossDomainMessengerABI.Pack(
		"relayMessage",
		big.NewInt(0),                // nonce
		testutils.RandomAddress(rng), // sender
		testutils.RandomAddress(rng), // target
		big.NewInt(0),                // value
		big.NewInt(0),                // min gas limit
		crossDomainMessageBz,         // message
	)
	require.NoError(t, err)

	to := testutils.RandomAddress(rng)
	depositTx := &gethtypes.DepositTx{
		// L2 aliased L1CrossDomainMessenger proxy address
		From: crossdomain.ApplyL1ToL2Alias(common.HexToAddress("0x3d609De69E066F85C38AC274e3EeC251EcfDeAa1")),
		To:   &to,
		Data: relayMessageBz,
	}
	return gethtypes.NewTx(depositTx)
}

func TxToBytes(t *testing.T, tx *gethtypes.Transaction) []byte {
	txBytes, err := tx.MarshalBinary()
	require.NoError(t, err)
	return txBytes
}

func cosmosTxsFromEthTxs(t *testing.T, l1InfoTx *gethtypes.Transaction, depositTxs, cosmosEthTxs []*gethtypes.Transaction) bfttypes.Txs {
	l1InfoTxBytes, err := l1InfoTx.MarshalBinary()
	require.NoError(t, err)
	ethTxBytes := []hexutil.Bytes{l1InfoTxBytes}
	for _, depositTx := range depositTxs {
		depositTxBytes, err := depositTx.MarshalBinary()
		require.NoError(t, err)
		ethTxBytes = append(ethTxBytes, depositTxBytes)
	}
	for _, cosmosEthTx := range cosmosEthTxs {
		cosmosEthTxBytes, err := cosmosEthTx.MarshalBinary()
		require.NoError(t, err)
		ethTxBytes = append(ethTxBytes, cosmosEthTxBytes)
	}
	cosmosTxs, err := monomer.AdaptPayloadTxsToCosmosTxs(ethTxBytes)
	require.NoError(t, err)
	return cosmosTxs
}

func GenerateBlockFromEthTxs(t *testing.T, l1InfoTx *gethtypes.Transaction, depositTxs, cosmosEthTxs []*gethtypes.Transaction) *monomer.Block {
	cosmosTxs := cosmosTxsFromEthTxs(t, l1InfoTx, depositTxs, cosmosEthTxs)
	block, err := monomer.MakeBlock(&monomer.Header{}, cosmosTxs)
	require.NoError(t, err)
	return block
}

// GenerateBlock generates a valid block (up to stateless validation). The block is not meant to be executed.
func GenerateBlock(t *testing.T) *monomer.Block {
	l1InfoTx, depositTx, cosmosEthTx := GenerateEthTxs(t)
	return GenerateBlockFromEthTxs(t, l1InfoTx, []*gethtypes.Transaction{depositTx}, []*gethtypes.Transaction{cosmosEthTx})
}

// GenerateBlockWithParentAndTxs generates a child block of parent with the cosmosTxs appended to the end of its transaction list.
// The genesis block is created if parent is nil.
func GenerateBlockWithParentAndTxs(t *testing.T, parent *monomer.Header, cosmosTxs ...bfttypes.Tx) *monomer.Block {
	l1InfoTx, _, _ := GenerateEthTxs(t)
	h := &monomer.Header{}
	if parent != nil {
		h.ParentHash = parent.Hash
		h.Height = parent.Height + 1
	}
	block, err := monomer.MakeBlock(h, append(cosmosTxsFromEthTxs(t, l1InfoTx, nil, nil), cosmosTxs...))
	require.NoError(t, err)
	return block
}

func GenerateL1Block() *gethtypes.Block {
	return gethtypes.NewBlock(&gethtypes.Header{
		BaseFee:    big.NewInt(10),
		Difficulty: common.Big0,
		Number:     big.NewInt(0),
		Time:       uint64(0),
	}, &gethtypes.Body{}, nil, trie.NewStackTrie(nil))
}

func convertPrivKey(ecdsaPrivKey *ecdsa.PrivateKey) *secp256k1.PrivateKey {
	privKeyBytes := ecdsaPrivKey.D.Bytes()
	var key secp256k1.ModNScalar
	if len(privKeyBytes) > 32 || key.SetByteSlice(privKeyBytes) {
		panic("overflow")
	}
	if key.IsZero() {
		panic("private keys must not be 0")
	}
	return secp256k1.NewPrivateKey(&key)
}

func BuildSDKTx(t *testing.T, chainID string, seqNum, accNum uint64, ethPrivKey *ecdsa.PrivateKey, msgs []protov1.Message) *sdktx.Tx {
	cosmosPrivKey := &cosmossecp256k1.PrivKey{
		Key: convertPrivKey(ethPrivKey).Serialize(),
	}

	var msgAnys []*codectypes.Any
	for _, msg := range msgs {
		msgAny, err := codectypes.NewAnyWithValue(msg)
		require.NoError(t, err)
		msgAnys = append(msgAnys, msgAny)
	}

	pubKeyAny, err := codectypes.NewAnyWithValue(cosmosPrivKey.PubKey())
	require.NoError(t, err)

	tx := &sdktx.Tx{
		Body: &sdktx.TxBody{
			Messages: msgAnys,
		},
		AuthInfo: &sdktx.AuthInfo{
			SignerInfos: []*sdktx.SignerInfo{
				{
					PublicKey: pubKeyAny,
					ModeInfo: &sdktx.ModeInfo{
						Sum: &sdktx.ModeInfo_Single_{
							Single: &sdktx.ModeInfo_Single{
								Mode: signing.SignMode_SIGN_MODE_DIRECT,
							},
						},
					},
					Sequence: seqNum,
				},
			},
			Fee: &sdktx.Fee{
				Amount:   sdktypes.NewCoins(sdktypes.NewCoin(rolluptypes.WEI, math.NewInt(100000000))),
				GasLimit: 1000000,
			},
		},
	}

	bodyBytes, err := protov1.Marshal(tx.Body)
	require.NoError(t, err)
	authInfoBytes, err := protov1.Marshal(tx.AuthInfo)
	require.NoError(t, err)

	signBytes, err := (proto.MarshalOptions{Deterministic: true}).Marshal(&txv1beta1.SignDoc{
		BodyBytes:     bodyBytes,
		AuthInfoBytes: authInfoBytes,
		ChainId:       chainID,
		AccountNumber: accNum,
	})
	require.NoError(t, err)

	signature, err := cosmosPrivKey.Sign(signBytes)
	require.NoError(t, err)

	tx.Signatures = [][]byte{signature}

	return tx
}
