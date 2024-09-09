package e2e

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/ethclient/gethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type L1Client struct {
	client *rpc.Client
	*ethclient.Client
}

func NewL1Client(client *rpc.Client) *L1Client {
	return &L1Client{
		client: client,
		Client: ethclient.NewClient(client),
	}
}

type MonomerClient struct {
	client *rpc.Client
	// We don't embed the ethclient.Client and gethclient.Client structs because Monomer doesn't implement the full geth `eth_*` interface.
	ethclient  *ethclient.Client
	gethclient *gethclient.Client
}

func NewMonomerClient(client *rpc.Client) *MonomerClient {
	return &MonomerClient{
		client:     client,
		ethclient:  ethclient.NewClient(client),
		gethclient: gethclient.New(client),
	}
}

func (m *MonomerClient) GenesisHash(ctx context.Context) (common.Hash, error) {
	type rpcBlock struct {
		Hash common.Hash `json:"hash"`
	}
	block := new(rpcBlock)
	if err := m.client.CallContext(ctx, &block, "eth_getBlockByNumber", "0x1", false); err != nil {
		return common.Hash{}, fmt.Errorf("eth_getBlockByNumber: %v", err)
	}
	return block.Hash, nil
}

func (m *MonomerClient) BlockByNumber(ctx context.Context, number *big.Int) (*ethtypes.Block, error) {
	block, err := m.ethclient.BlockByNumber(ctx, number)
	if err != nil {
		return nil, err
	}
	return block, nil
}

// GetProof returns the account and storage values of the specified account including the Merkle-proof.
// The block number can be nil, in which case the value is taken from the latest known block.
func (m *MonomerClient) GetProof(
	ctx context.Context,
	account common.Address,
	keys []string,
	blockNumber *big.Int,
) (*gethclient.AccountResult, error) {
	accountResult, err := m.gethclient.GetProof(ctx, account, keys, blockNumber)
	if err != nil {
		return nil, fmt.Errorf("get proof: %v", err)
	}
	return accountResult, nil
}
