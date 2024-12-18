package monomer

import (
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strconv"
	"time"

	abcitypes "github.com/cometbft/cometbft/abci/types"
	bfttypes "github.com/cometbft/cometbft/types"
	"github.com/cosmos/cosmos-sdk/types/bech32"
	opeth "github.com/ethereum-optimism/optimism/op-service/eth"
	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto/secp256k1"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer/utils"
	"golang.org/x/crypto/ripemd160" //nolint:staticcheck
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
	return (*hexutil.Big)(id.Big())
}

func (id ChainID) Big() *big.Int {
	return new(big.Int).SetUint64(uint64(id))
}

type Header struct {
	ChainID          ChainID
	Height           uint64
	Time             uint64
	ParentHash       common.Hash
	StateRoot        common.Hash
	ParentBeaconRoot *common.Hash
	GasLimit         uint64
	Hash             common.Hash
}

func (h *Header) ToComet() *bfttypes.Header {
	return &bfttypes.Header{
		ChainID:     h.ChainID.String(),
		Height:      int64(h.Height),
		Time:        time.Unix(int64(h.Time), 0),
		LastBlockID: bfttypes.BlockID{Hash: h.ParentHash.Bytes()},
		AppHash:     h.StateRoot.Bytes(),
	}
}

type Block struct {
	Header *Header
	Txs    bfttypes.Txs
}

// NewBlock creates a new block. The header and txs must be non-nil. It performs no other validation.
func NewBlock(h *Header, txs bfttypes.Txs) *Block {
	if h == nil || txs == nil {
		panic("header or txs is nil")
	}
	return &Block{
		Header: h,
		Txs:    txs,
	}
}

// SetHeader calculates the extrinsic properties on the header (like the block hash) and resets them.
// It assumes the block has been created with NewBlock.
func SetHeader(block *Block) (*Block, error) {
	ethBlock, err := block.ToEth()
	if err != nil {
		return nil, fmt.Errorf("convert block to Ethereum representation: %v", err)
	}
	block.Header.Hash = ethBlock.Hash()
	return block, nil
}

// MakeBlock creates a block and calculates the extrinsic properties on the header (like the block hash).
func MakeBlock(h *Header, txs bfttypes.Txs) (*Block, error) {
	return SetHeader(NewBlock(h, txs))
}

// ToEth converts a partial Monomer Header to an Ethereum Header.
// Extrinsic properties on the header (like the block hash) need to be set separately by SetHeader.
func (h *Header) ToEth() *ethtypes.Header {
	return &ethtypes.Header{
		ParentHash:       h.ParentHash,
		Root:             h.StateRoot,
		Number:           new(big.Int).SetUint64(h.Height),
		GasLimit:         h.GasLimit,
		MixDigest:        common.Hash{},
		Time:             h.Time,
		UncleHash:        ethtypes.EmptyUncleHash,
		ReceiptHash:      ethtypes.EmptyReceiptsHash,
		BaseFee:          common.Big0,
		WithdrawalsHash:  &ethtypes.EmptyWithdrawalsHash,
		Difficulty:       common.Big0,
		ParentBeaconRoot: h.ParentBeaconRoot,
	}
}

func (b *Block) ToEth() (*ethtypes.Block, error) {
	if b == nil {
		return nil, errors.New("converted a nil block")
	}

	txs, err := AdaptCosmosTxsToEthTxs(b.Txs)
	if err != nil {
		return nil, fmt.Errorf("adapt txs: %v", err)
	}
	return ethtypes.NewBlock(
		b.Header.ToEth(),
		&ethtypes.Body{
			Transactions: txs,
			// op-node version requires non-nil withdrawals when it derives attributes from L1,
			// so unsafe block consolidation will fail if we have nil withdrawals here.
			Withdrawals: []*ethtypes.Withdrawal{},
		},
		[]*ethtypes.Receipt{},
		trie.NewStackTrie(nil),
	), nil
}

func (b *Block) ToCometLikeBlock() *bfttypes.Block {
	return &bfttypes.Block{
		Header: *b.Header.ToComet(),
		Data: bfttypes.Data{
			Txs: b.Txs,
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

func NewChainConfig(chainID *big.Int) *params.ChainConfig {
	return &params.ChainConfig{
		ChainID: chainID,

		ByzantiumBlock:      new(big.Int),
		ConstantinopleBlock: new(big.Int),
		PetersburgBlock:     new(big.Int),
		IstanbulBlock:       new(big.Int),
		MuirGlacierBlock:    new(big.Int),
		BerlinBlock:         new(big.Int),
		LondonBlock:         new(big.Int),
		ArrowGlacierBlock:   new(big.Int),
		GrayGlacierBlock:    new(big.Int),
		MergeNetsplitBlock:  new(big.Int),

		BedrockBlock: new(big.Int),
		RegolithTime: utils.Ptr(uint64(0)),
		CanyonTime:   utils.Ptr(uint64(0)),
	}
}

// CosmosETHAddress is a Cosmos address packed into an Ethereum address.
// Only addresses derived from secp256k1 keys can be packed into an Ethereum address.
// See [ADR-28] for more details.
//
//nolint:lll // [ADR-28]: https://github.com/cosmos/cosmos-sdk/blob/8bfcf554275c1efbb42666cc8510d2da139b67fa/docs/architecture/adr-028-public-key-addresses.md?plain=1#L85
type CosmosETHAddress common.Address

// PubkeyToCosmosETHAddress converts a secp256k1 public key to a CosmosETHAddress.
// Passing in a non-secp256k1 key results in undefined behavior.
func PubkeyToCosmosETHAddress(pubKey *ecdsa.PublicKey) CosmosETHAddress {
	sha := sha256.Sum256(secp256k1.CompressPubkey(pubKey.X, pubKey.Y))
	hasherRIPEMD160 := ripemd160.New()
	if _, err := hasherRIPEMD160.Write(sha[:]); err != nil {
		// hash.Hash never returns an error on Write. This panic should never execute.
		panic(fmt.Errorf("ripemd160: %v", err))
	}
	return CosmosETHAddress(hasherRIPEMD160.Sum(nil))
}

func (a CosmosETHAddress) Encode(hrp string) (string, error) {
	return bech32.ConvertAndEncode(hrp, common.Address(a).Bytes())
}
