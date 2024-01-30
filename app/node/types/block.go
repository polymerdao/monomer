package types

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math/big"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/trie"
)

var (
	EmptyBloom      = types.Bloom(make([]byte, types.BloomByteLength))
	EmptyExtra      = make([]byte, 0)
	HashOfEmptyHash = sha256.Sum256([]byte{})
	ZeroHash        = Hash{}
)

type Bytes []byte

// This is only a representation of the block from the block-store point of view
// The interface is missing key accessors to fields within the block but that are not needed
// by the block store.
type BlockData interface {
	Height() int64
	Bytes() []byte
	Hash() Hash
	ParentHash() Hash
}

type Header struct {
	ChainID string `json:"chain_id"`
	Height  int64  `json:"height"`
	Time    uint64 `json:"time"`

	// prev block hash
	LastBlockHash []byte `json:"last_block_hash"`

	// hashes of block data
	LastCommitHash Bytes `json:"last_commit_hash"` // commit from validators from the last block
	DataHash       Bytes `json:"data_hash"`        // transactions

	// hashes from the app output from the prev block
	ValidatorsHash     Bytes `json:"validators_hash"`      // validators for the current block
	NextValidatorsHash Bytes `json:"next_validators_hash"` // validators for the next block
	ConsensusHash      Bytes `json:"consensus_hash"`       // consensus params for current block
	AppHash            Bytes `json:"app_hash"`             // state after txs from the previous block
	// root hash of all results from the txs from the previous block
	LastResultsHash Bytes `json:"last_results_hash"`

	// consensus info
	EvidenceHash Bytes `json:"evidence_hash"` // evidence included in the block
}

func (h *Header) Populate(cosmosHeader *tmproto.Header) *Header {
	h.ChainID = cosmosHeader.ChainID
	h.Height = cosmosHeader.Height
	h.Time = uint64(cosmosHeader.Time.Unix())
	h.LastBlockHash = cosmosHeader.LastBlockId.Hash
	h.LastCommitHash = cosmosHeader.LastCommitHash
	h.DataHash = cosmosHeader.DataHash
	h.ValidatorsHash = cosmosHeader.ValidatorsHash
	h.NextValidatorsHash = cosmosHeader.NextValidatorsHash
	h.ConsensusHash = cosmosHeader.ConsensusHash
	h.AppHash = cosmosHeader.AppHash
	h.LastResultsHash = cosmosHeader.LastResultsHash
	h.EvidenceHash = cosmosHeader.EvidenceHash
	return h
}

type Block struct {
	Txs             bfttypes.Txs       `json:"txs"`
	Header          *Header            `json:"header"`
	ParentBlockHash Hash               `json:"parentHash"`
	L1Txs           []eth.Data         `json:"l1Txs"`
	GasLimit        hexutil.Uint64     `json:"gasLimit"`
	BlockHash       Hash               `json:"hash"`
	PrevRandao      eth.Bytes32        `json:"prevRandao"`
	Withdrawals     *types.Withdrawals `json:"withdrawals,omitempty"`
}

var _ BlockData = (*Block)(nil)

func BlockUnmarshaler(bytes []byte) (BlockData, error) {
	b := Block{}
	if err := json.Unmarshal(bytes, &b); err != nil {
		panic(err)
	}
	return &b, nil
}

func (b *Block) Height() int64 {
	return b.Header.Height
}

func (b *Block) Bytes() []byte {
	bytes, err := json.Marshal(b)
	if err != nil {
		panic(err)
	}
	return bytes
}

// Hash returns a unique hash of the block, used as the block identifier
func (b *Block) Hash() Hash {
	if b.BlockHash == ZeroHash {
		header := types.Header{}
		header.ParentHash = b.ParentHash()
		header.Root = common.BytesToHash(b.Header.AppHash)
		header.Number = big.NewInt(b.Height())
		header.GasLimit = uint64(b.GasLimit)
		header.MixDigest = common.Hash(b.PrevRandao)
		header.Time = b.Header.Time
		_, header.TxHash = b.Transactions()

		// these are set to "empty" stuff but they are needed to corre
		// a correct
		header.UncleHash = types.EmptyUncleHash
		header.ReceiptHash = types.EmptyReceiptsHash
		header.BaseFee = common.Big0
		header.WithdrawalsHash = &types.EmptyWithdrawalsHash

		hash := header.Hash()
		copy(b.BlockHash[:], hash[:])
	}
	return b.BlockHash
}

func (b *Block) ParentHash() Hash {
	return b.ParentBlockHash
}

func (b *Block) Transactions() (types.Transactions, Hash) {
	var txs types.Transactions
	for _, l1tx := range b.L1Txs {
		var tx types.Transaction
		if err := tx.UnmarshalBinary(l1tx); err != nil {
			panic("failed to unmarshal l2 txs")
		}
		// TODO add other fields?
		txs = append(txs, &tx)
	}
	chainId, ok := big.NewInt(0).SetString(b.Header.ChainID, 10)
	if !ok {
		panic(fmt.Sprintf("block chain id is not an integer %s", b.Header.ChainID))
	}

	for _, l2tx := range b.Txs {
		//TODO: update to use proper Gas and To values if possible
		txData := &types.DynamicFeeTx{
			ChainID: chainId,
			Data:    l2tx,
			Gas:     0,
			Value:   big.NewInt(0),
			To:      nil,
		}
		tx := types.NewTx(txData)
		txs = append(txs, tx)
	}
	return txs, types.DeriveSha(txs, trie.NewStackTrie(nil))
}

// This trick is played by the eth rpc server too. Instead of constructing
// an actual eth block, simply create a map with the right keys so the client
// can unmarshal it into a block
func (b *Block) ToEthLikeBlock(inclTx bool) map[string]any {
	result := map[string]any{
		// These are the ones that make sense to polymer.
		"parentHash": b.ParentHash(),
		"stateRoot":  common.BytesToHash(b.Header.AppHash),
		"number":     (*hexutil.Big)(big.NewInt(b.Height())),
		"gasLimit":   b.GasLimit,
		"mixHash":    b.PrevRandao,
		"timestamp":  hexutil.Uint64(b.Header.Time),
		"hash":       b.Hash(),

		// these are required fields that need to be part of the header or
		// the eth client will complain during unmarshalling
		"sha3Uncles":      types.EmptyUncleHash,
		"receiptsRoot":    types.EmptyReceiptsHash,
		"baseFeePerGas":   (*hexutil.Big)(common.Big0),
		"difficulty":      (*hexutil.Big)(common.Big0),
		"extraData":       EmptyExtra,
		"gasUsed":         hexutil.Uint64(0),
		"logsBloom":       EmptyBloom,
		"withdrawalsRoot": types.EmptyWithdrawalsHash,
		"withdrawals":     b.Withdrawals,
	}

	if inclTx {
		txs, hash := b.Transactions()
		result["transactionsRoot"] = hash
		result["transactions"] = txs
	} else {
		result["transactionsRoot"] = types.EmptyTxsHash
	}
	return result
}

func MustInferBlock(b BlockData) *Block {
	block, ok := b.(*Block)
	if !ok {
		panic("could not infer block from BlockData")
	}
	return block
}
