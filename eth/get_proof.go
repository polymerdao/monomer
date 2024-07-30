package eth

import (
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/polymerdao/monomer/app/peptide/store"
)

const commonHashLength = 32

type ProofProvider struct {
	database   state.Database
	blockStore store.BlockStoreReader
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

func NewProofProvider(db state.Database, blockStore store.BlockStoreReader) *ProofProvider {
	return &ProofProvider{
		database:   db,
		blockStore: blockStore,
	}
}

// getState returns the state.StateBD and Header of block at the given number.
// If the passed number is nil, it returns the the db and header of the latest block.
func (p *ProofProvider) getState(blockNumber *big.Int) (*state.StateDB, types.Header, error) {
	var ethBlock *types.Block
	var err error

	if blockNumber == nil {
		ethBlock, err = p.blockStore.HeadBlock().ToEth()
	} else {
		ethBlock, err = p.blockStore.BlockByNumber(blockNumber.Int64()).ToEth()
	}

	if err != nil {
		return nil, types.Header{}, fmt.Errorf("getting eth block %d: %w", blockNumber, err)
	}

	header := ethBlock.Header()
	hash := ethBlock.Hash()

	sdb, err := state.New(hash, p.database, nil)
	if err != nil {
		return nil, *header, fmt.Errorf("opening state.StateDB: %w", err)
	}

	return sdb, *header, nil
}

// decodeHash parses a hex-encoded 32-byte hash. The input may optionally
// be prefixed by 0x and can have a byte length up to 32.
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

// AccountResult is the result of a GetProof operation.
type AccountResult struct {
	Address      common.Address  `json:"address"`
	AccountProof []string        `json:"accountProof"`
	Balance      *big.Int        `json:"balance"`
	CodeHash     common.Hash     `json:"codeHash"`
	Nonce        uint64          `json:"nonce"`
	StorageHash  common.Hash     `json:"storageHash"`
	StorageProof []StorageResult `json:"storageProof"`
}

// StorageResult provides a proof for a key-value pair.
type StorageResult struct {
	Key   string   `json:"key"`
	Value *big.Int `json:"value"`
	Proof []string `json:"proof"`
}

// GetProof returns the account and storage values of the specified account including the Merkle-proof.
// The block number can be nil, in which case the value is taken from the latest known block.
//
// implementation copied, with light modifications, from
// https://github.com/ethereum-optimism/op-geth/blob/f2e69450c6eec9c35d56af91389a1c47737206ca/ethclient/gethclient/gethclient.go#L83
// (HEAD @ 2024-07-29)
func (p *ProofProvider) GetProof(account common.Address, keys []string, blockNumber *big.Int) (*AccountResult, error) {
	// Avoid keys being 'null'.
	if keys == nil {
		keys = []string{}
	}

	//////////
	// Start: Monomer-specific modifications
	//////////

	// a call against an internally exposed API - ec.c.CallContext(ctx, &res, "eth_getProof", account, keys, toBlockNumArg(blockNumber))
	// is replaced with a call to an adjacent function implementation
	res, err := p.getProof(account, keys, blockNumber)
	if err != nil {
		return nil, err
	}

	//////////
	// End: Monomer-specific modifications
	//////////

	// Turn hexutils back to normal datatypes
	storageResults := make([]StorageResult, 0, len(res.StorageProof))
	for _, st := range res.StorageProof {
		storageResults = append(storageResults, StorageResult{
			Key:   st.Key,
			Value: st.Value.ToInt(),
			Proof: st.Proof,
		})
	}

	result := AccountResult{
		Address:      res.Address,
		AccountProof: res.AccountProof,
		Balance:      res.Balance.ToInt(),
		Nonce:        uint64(res.Nonce),
		CodeHash:     res.CodeHash,
		StorageHash:  res.StorageHash,
		StorageProof: storageResults,
	}
	return &result, err
}

///////
//
// geth.INTERNAL functionality
//
// todo: move to an `eth.internal` folder?
//
////////

// GetProof returns the Merkle-proof for a given account and optionally some storage keys.
//
// implementation copied, with light modifications, from
// https://github.com/ethereum-optimism/op-geth/blob/f2e69450c6eec9c35d56af91389a1c47737206ca/internal/ethapi/api.go#L708
// (HEAD @ 2024-07-29)
func (p *ProofProvider) getProof(address common.Address, storageKeys []string, blockNumber *big.Int) (*accountResult, error) {
	var (
		keys         = make([]common.Hash, len(storageKeys))
		keyLengths   = make([]int, len(storageKeys))
		storageProof = make([]storageResult, len(storageKeys))
	)
	// Deserialize all keys. This prevents state access on invalid input.
	for i, hexKey := range storageKeys {
		var err error
		keys[i], keyLengths[i], err = decodeHash(hexKey)
		if err != nil {
			return nil, err
		}
	}

	//////////
	// Start: Monomer-specific modifications
	//////////

	// the internal geth mechanism contained different hooks into block state data
	statedb, header, err := p.getState(blockNumber)

	//////////
	// End: Monomer-specific modifications
	//////////

	if statedb == nil || err != nil {
		return nil, err
	}
	codeHash := statedb.GetCodeHash(address)
	storageRoot := statedb.GetStorageRoot(address)

	if len(keys) > 0 {
		var storageTrie state.Trie
		if storageRoot != types.EmptyRootHash && storageRoot != (common.Hash{}) {
			id := trie.StorageTrieID(header.Root, crypto.Keccak256Hash(address.Bytes()), storageRoot)
			st, err := trie.NewStateTrie(id, statedb.Database().TrieDB())
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
				storageProof[i] = storageResult{outputKey, &hexutil.Big{}, []string{}}
				continue
			}
			var proof proofList
			if err := storageTrie.Prove(crypto.Keccak256(key.Bytes()), &proof); err != nil {
				return nil, err
			}
			value := (*hexutil.Big)(statedb.GetState(address, key).Big())
			storageProof[i] = storageResult{outputKey, value, proof}
		}
	}
	// Create the accountProof.
	tr, err := trie.NewStateTrie(trie.StateTrieID(header.Root), statedb.Database().TrieDB())
	if err != nil {
		return nil, err
	}
	var accountProof proofList
	if err := tr.Prove(crypto.Keccak256(address.Bytes()), &accountProof); err != nil {
		return nil, err
	}
	balance := statedb.GetBalance(address).ToBig()
	return &accountResult{
		Address:      address,
		AccountProof: accountProof,
		Balance:      (*hexutil.Big)(balance),
		CodeHash:     codeHash,
		Nonce:        hexutil.Uint64(statedb.GetNonce(address)),
		StorageHash:  storageRoot,
		StorageProof: storageProof,
	}, statedb.Error()
}

// accountResult structs for internal getProof method
type accountResult struct {
	Address      common.Address  `json:"address"`
	AccountProof []string        `json:"accountProof"`
	Balance      *hexutil.Big    `json:"balance"`
	CodeHash     common.Hash     `json:"codeHash"`
	Nonce        hexutil.Uint64  `json:"nonce"`
	StorageHash  common.Hash     `json:"storageHash"`
	StorageProof []storageResult `json:"storageProof"`
}

type storageResult struct {
	Key   string       `json:"key"`
	Value *hexutil.Big `json:"value"`
	Proof []string     `json:"proof"`
}
