package eth

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer"
	"github.com/polymerdao/monomer/eth/internal/ethapi"
)

const commonHashLength = 32

type ProofBlockDB interface {
	HeadBlock() (*monomer.Block, error)
	BlockByHeight(uint64) (*monomer.Block, error)
	BlockByHash(common.Hash) (*monomer.Block, error)
}

type ProofProvider struct {
	database   state.Database
	blockStore ProofBlockDB
}

// proofList implements ethdb.KeyValueWriter and collects the proofs as
// hex-strings for delivery to rpc-caller.
type proofList []string

func (n *proofList) Put(key, value []byte) error {
	*n = append(*n, hexutil.Encode(value))
	return nil
}

func (n *proofList) Delete(key []byte) error {
	panic("not supported")
}

func NewProofProvider(db state.Database, blockStore ProofBlockDB) *ProofProvider {
	return &ProofProvider{
		database:   db,
		blockStore: blockStore,
	}
}

// GetProof returns the account and storage values of the specified account including the Merkle-proof.
// The user can specify either a block number or a block hash to build to proof from.
//
// implementation copied, with light modifications, from
// https://github.com/ethereum-optimism/op-geth/blob/3653ceb/internal/ethapi/api.go#L708
func (p *ProofProvider) GetProof(
	address common.Address,
	storageKeys []string,
	blockNrOrHash rpc.BlockNumberOrHash,
) (*ethapi.AccountResult, error) {
	var (
		keys         = make([]common.Hash, len(storageKeys))
		keyLengths   = make([]int, len(storageKeys))
		storageProof = make([]ethapi.StorageResult, len(storageKeys))
	)
	// Deserialize all keys. This prevents state access on invalid input.
	for i, hexKey := range storageKeys {
		var err error
		keys[i], keyLengths[i], err = decodeHash(hexKey)
		if err != nil {
			return nil, err
		}
	}
	stateDB, stateRoot, err := p.getState(blockNrOrHash)
	if err != nil {
		return nil, fmt.Errorf("get state by block number or hash: %w", err)
	}
	codeHash := stateDB.GetCodeHash(address)
	storageRoot := stateDB.GetStorageRoot(address)

	if len(keys) > 0 {
		var storageTrie state.Trie
		if storageRoot != types.EmptyRootHash && storageRoot != (common.Hash{}) {
			id := trie.StorageTrieID(stateRoot, crypto.Keccak256Hash(address.Bytes()), storageRoot)
			st, err := trie.NewStateTrie(id, stateDB.Database().TrieDB())
			if err != nil {
				return nil, err
			}
			storageTrie = st
		}
		// Create the proofs for the storageKeys.
		for i, key := range keys {
			// Output key encoding is a bit special: if the input was a 32-byte hash, it is
			// returned as such. Otherwise, we apply the QUANTITY encoding mandated by the
			// JSON-RPC spec for getProof. This behavior exists to preserve backwards
			// compatibility with older client versions.
			var outputKey string
			if keyLengths[i] != commonHashLength {
				outputKey = hexutil.EncodeBig(key.Big())
			} else {
				outputKey = hexutil.Encode(key[:])
			}
			if storageTrie == nil {
				storageProof[i] = ethapi.StorageResult{Key: outputKey, Value: &hexutil.Big{}, Proof: []string{}}
				continue
			}
			var proof proofList
			if err := storageTrie.Prove(crypto.Keccak256(key.Bytes()), &proof); err != nil {
				return nil, err
			}
			value := (*hexutil.Big)(stateDB.GetState(address, key).Big())
			storageProof[i] = ethapi.StorageResult{Key: outputKey, Value: value, Proof: proof}
		}
	}
	// Create the accountProof.
	tr, err := trie.NewStateTrie(trie.StateTrieID(stateRoot), stateDB.Database().TrieDB())
	if err != nil {
		return nil, err
	}
	var accountProof proofList
	if err := tr.Prove(crypto.Keccak256(address.Bytes()), &accountProof); err != nil {
		return nil, err
	}
	balance := stateDB.GetBalance(address).ToBig()
	return &ethapi.AccountResult{
		Address:      address,
		AccountProof: accountProof,
		Balance:      (*hexutil.Big)(balance),
		CodeHash:     codeHash,
		Nonce:        hexutil.Uint64(stateDB.GetNonce(address)),
		StorageHash:  storageRoot,
		StorageProof: storageProof,
	}, stateDB.Error()
}

// getState returns the state.StateDB and state root of the Monomer block at the given block number or hash.
// If the passed block number is "latest", it returns the db and state root of the latest block.
func (p *ProofProvider) getState(blockNrOrHash rpc.BlockNumberOrHash) (*state.StateDB, common.Hash, error) {
	var (
		block *monomer.Block
		err   error
	)
	if blockNr, ok := blockNrOrHash.Number(); ok {
		if blockNr == rpc.LatestBlockNumber {
			block, err = p.blockStore.HeadBlock()
		} else if blockNr == rpc.EarliestBlockNumber || blockNr == rpc.PendingBlockNumber ||
			blockNr == rpc.SafeBlockNumber || blockNr == rpc.FinalizedBlockNumber {
			return nil, common.Hash{}, fmt.Errorf("\"%s\" is not supported; either use \"latest\" or a valid block number or hash", blockNr)
		} else {
			block, err = p.blockStore.BlockByHeight(uint64(blockNr))
		}
		if err != nil {
			return nil, common.Hash{}, fmt.Errorf("get block by height (%d): %w", blockNr, err)
		}
	} else if hash, ok := blockNrOrHash.Hash(); ok {
		block, err = p.blockStore.BlockByHash(hash)
		if err != nil {
			return nil, common.Hash{}, fmt.Errorf("get block by hash (%s): %w", hash, err)
		}
	}

	if block == nil {
		return nil, common.Hash{}, fmt.Errorf("block %w", ethereum.NotFound)
	}
	header := block.Header
	if header == nil {
		return nil, common.Hash{}, fmt.Errorf("header %w", ethereum.NotFound)
	}

	stateDB, err := state.New(header.StateRoot, p.database, nil)
	if err != nil {
		return nil, common.Hash{}, fmt.Errorf("opening state.StateDB: %w", err)
	}
	if stateDB == nil {
		return nil, common.Hash{}, fmt.Errorf("stateDB %w", ethereum.NotFound)
	}

	return stateDB, header.StateRoot, nil
}

// decodeHash parses a hex-encoded 32-byte hash. The input may optionally
// be prefixed by 0x and can have a byte length up to 32.
//
// implementation copied, with light modifications, from
// https://github.com/ethereum-optimism/op-geth/blob/3653ceb/internal/ethapi/api.go#L802
func decodeHash(s string) (h common.Hash, inputLength int, err error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	if (len(s) & 1) > 0 {
		s = "0" + s
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return common.Hash{}, 0, errors.New("hex string invalid")
	}
	if len(b) > commonHashLength {
		return common.Hash{}, len(b), errors.New("hex string too long, want at most 32 bytes")
	}
	return common.BytesToHash(b), len(b), nil
}
