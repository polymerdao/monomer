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
	rolluptypes "github.com/polymerdao/monomer/x/rollup/types"
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

// MakeBlock creates a new block. It calculates stateless properties on the header (like the block hash) and resets them.
// The header must be non-nil. The txs may be nil.
func MakeBlock(h *Header, txs bfttypes.Txs) (*Block, error) {
	block := &Block{
		Header: h,
		Txs:    txs,
	}
	ethBlock, err := block.ToEth()
	if err != nil {
		return nil, fmt.Errorf("convert block to Ethereum representation: %v", err)
	}
	block.Header.Hash = ethBlock.Hash()
	return block, nil
}

func (b *Block) ToEth() (*ethtypes.Block, error) {
	txs, err := rolluptypes.AdaptCosmosTxsToEthTxs(b.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt txs: %v", err)
	}
	return ethtypes.NewBlock(
		&ethtypes.Header{
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
		},
		txs,
		nil,
		[]*ethtypes.Receipt{},
		trie.NewStackTrie(nil),
	), nil
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
	if p.NoTxPool || len(p.CosmosTxs) == 0 {
		hashDataAsBinary(hasher, p.NoTxPool)
		hashDataAsBinary(hasher, uint64(len(p.CosmosTxs)))
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
