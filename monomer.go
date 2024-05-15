package monomer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"math/big"
	"strconv"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bftbytes "github.com/cometbft/cometbft/libs/bytes"
	bfttypes "github.com/cometbft/cometbft/types"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

type Application interface {
	Info(context.Context, *abcitypes.RequestInfo) (*abcitypes.ResponseInfo, error)
	Query(context.Context, *abcitypes.RequestQuery) (*abcitypes.ResponseQuery, error)

	CheckTx(context.Context, *abcitypes.RequestCheckTx) (*abcitypes.ResponseCheckTx, error)

	InitChain(context.Context, *abcitypes.RequestInitChain) (*abcitypes.ResponseInitChain, error)
	FinalizeBlock(context.Context, *abcitypes.RequestFinalizeBlock) (*abcitypes.ResponseFinalizeBlock, error)
	Commit(context.Context, *abcitypes.RequestCommit) (*abcitypes.ResponseCommit, error)

	RollbackToHeight(context.Context, uint64) error
}

// CosmosTxAdapter transforms Cosmos transactions into Ethereum transactions.
//
// In practice, this will use msg types from Monomer's rollup module, but importing the rollup module here would create a circular module
// dependency between Monomer, the SDK, and the rollup module. sdk -> monomer -> rollup -> sdk, where -> is "depends on".
type CosmosTxAdapter func(cosmosTxs bfttypes.Txs) (ethtypes.Transactions, error)

// PayloadTxAdapter transforms Op payload transactions into Cosmos transactions.
//
// In practice, this will use msg types from Monomer's rollup module, but importing the rollup module here would create a circular module
// dependency between Monomer, the SDK, and the rollup module. sdk -> monomer -> rollup -> sdk, where -> is "depends on".
type PayloadTxAdapter func(ethTxs []hexutil.Bytes) (bfttypes.Txs, error)

type ChainID uint64

func (id ChainID) String() string {
	return strconv.FormatUint(uint64(id), 10)
}

func (id ChainID) HexBig() *hexutil.Big {
	return (*hexutil.Big)(new(big.Int).SetUint64(uint64(id)))
}

type Header struct {
	ChainID    ChainID     `json:"chain_id"`
	Height     int64       `json:"height"`
	Time       uint64      `json:"time"`
	ParentHash common.Hash `json:"parentHash"`
	// state after txs from the *previous* block
	AppHash  []byte      `json:"app_hash"`
	GasLimit uint64      `json:"gasLimit"`
	Hash     common.Hash `json:"hash"`
}

func (h *Header) ToComet() *bfttypes.Header {
	return &bfttypes.Header{
		ChainID: h.ChainID.String(),
		Height:  h.Height,
		Time:    time.Unix(int64(h.Time), 0),
		AppHash: h.AppHash,
	}
}

type Block struct {
	Header *Header      `json:"header"`
	Txs    bfttypes.Txs `json:"txs"`
}

// Hash returns a unique hash of the block, used as the block identifier
func (b *Block) Hash() common.Hash {
	if b.Header.Hash == (common.Hash{}) {
		// We exclude the tx commitment.
		// TODO better hashing technique than using Ethereum's.
		b.Header.Hash = (&ethtypes.Header{
			ParentHash:      b.Header.ParentHash,
			Root:            common.BytesToHash(b.Header.AppHash), // TODO actually take the keccak
			Number:          big.NewInt(b.Header.Height),
			GasLimit:        b.Header.GasLimit,
			MixDigest:       common.Hash{},
			Time:            b.Header.Time,
			UncleHash:       ethtypes.EmptyUncleHash,
			ReceiptHash:     ethtypes.EmptyReceiptsHash,
			BaseFee:         common.Big0,
			WithdrawalsHash: &ethtypes.EmptyWithdrawalsHash,
		}).Hash()
	}
	return b.Header.Hash
}

// This trick is played by the eth rpc server too. Instead of constructing
// an actual eth block, simply create a map with the right keys so the client
// can unmarshal it into a block
func (b *Block) ToEthLikeBlock(txs ethtypes.Transactions, inclTxs bool) map[string]any {
	excessBlobGas := hexutil.Uint64(0)
	blockGasUsed := hexutil.Uint64(0)
	result := map[string]any{
		// These are the ones that make sense to polymer.
		"parentHash": b.Header.ParentHash,
		"stateRoot":  common.BytesToHash(b.Header.AppHash),
		"number":     (*hexutil.Big)(big.NewInt(b.Header.Height)),
		"gasLimit":   hexutil.Uint64(b.Header.GasLimit),
		"mixHash":    common.Hash{},
		"timestamp":  hexutil.Uint64(b.Header.Time),
		"hash":       b.Hash(),

		// these are required fields that need to be part of the header or
		// the eth client will complain during unmarshalling
		"sha3Uncles":            ethtypes.EmptyUncleHash,
		"receiptsRoot":          ethtypes.EmptyReceiptsHash,
		"baseFeePerGas":         (*hexutil.Big)(common.Big0),
		"difficulty":            (*hexutil.Big)(common.Big0),
		"extraData":             []byte{},
		"gasUsed":               hexutil.Uint64(0),
		"logsBloom":             ethtypes.Bloom(make([]byte, ethtypes.BloomByteLength)),
		"withdrawalsRoot":       ethtypes.EmptyWithdrawalsHash,
		"withdrawals":           ethtypes.Withdrawals{},
		"blobGasUsed":           &blockGasUsed,
		"excessBlobGas":         &excessBlobGas,
		"parentBeaconBlockRoot": common.Hash{},
		"transactionsRoot":      ethtypes.DeriveSha(txs, trie.NewStackTrie(nil)),
	}
	if inclTxs {
		result["transactions"] = txs
	}
	return result
}

func (b *Block) ToCometLikeBlock() *bfttypes.Block {
	return &bfttypes.Block{
		Header: bfttypes.Header{
			ChainID: b.Header.ChainID.String(),
			Time:    time.Unix(int64(b.Header.Time), 0),
			Height:  b.Header.Height,
			AppHash: bftbytes.HexBytes(b.Header.AppHash),
		},
	}
}

type PayloadAttributes struct {
	Timestamp             uint64
	PrevRandao            [32]byte
	SuggestedFeeRecipient common.Address
	Withdrawals           *ethtypes.Withdrawals
	NoTxPool              bool
	GasLimit              uint64
	ParentBeaconBlockRoot *common.Hash
	ParentHash            common.Hash
	Height                int64
	Transactions          []hexutil.Bytes
	CosmosTxs             bfttypes.Txs
	id                    *engine.PayloadID
}

// ID returns a PaylodID (a hash) from a PayloadAttributes when it's applied to a head block.
// Hashing does not conform to go-ethereum/miner/payload_building.go
// PayloadID is only calculated once, and cached for future calls.
func (p *PayloadAttributes) ID() *engine.PayloadID {
	if p.id != nil {
		return p.id
	}
	hasher := sha256.New()

	hashData(hasher, p.ParentHash[:])
	hashDataAsBinary(hasher, p.Timestamp)
	hashData(hasher, p.PrevRandao[:])
	hashData(hasher, p.SuggestedFeeRecipient[:])
	hashDataAsBinary(hasher, p.GasLimit)
	if p.NoTxPool || len(p.Transactions) == 0 {
		hashDataAsBinary(hasher, p.NoTxPool)
		hashDataAsBinary(hasher, uint64(len(p.Transactions)))
		for _, txData := range p.CosmosTxs {
			hashData(hasher, txData)
		}
	}

	var out engine.PayloadID
	copy(out[:], hasher.Sum(nil)[:8])
	p.id = &out
	return &out
}

func hashData(h hash.Hash, data []byte) {
	// We know hash.Hash should never return an error, so a panic is fine.
	if _, err := h.Write(data); err != nil {
		panic(fmt.Errorf("hash data: %v", err))
	}
}

func hashDataAsBinary(h hash.Hash, data any) {
	// We know hash.Hash should never return an error, so a panic is fine.
	if err := binary.Write(h, binary.BigEndian, data); err != nil {
		panic(fmt.Errorf("hash data as binary: %v", err))
	}
}

// ValidForkchoiceUpdateResult returns a valid ForkchoiceUpdateResult with given head block hash.
func ValidForkchoiceUpdateResult(headBlockHash *common.Hash, id *engine.PayloadID) *opeth.ForkchoiceUpdatedResult {
	return &opeth.ForkchoiceUpdatedResult{
		PayloadStatus: opeth.PayloadStatusV1{
			Status:          opeth.ExecutionValid,
			LatestValidHash: headBlockHash,
		},
		PayloadID: id,
	}
}
