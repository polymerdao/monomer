package e2e

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
)

type AnvilClient struct {
	client *rpc.Client
	*ethclient.Client
}

func NewAnvilClient(client *rpc.Client) *AnvilClient {
	return &AnvilClient{
		client: client,
		Client: ethclient.NewClient(client),
	}
}

func (a *AnvilClient) SetBalance(ctx context.Context, account common.Address, balance uint64) error {
	if err := a.client.CallContext(ctx, nil, "anvil_setBalance", account, hexutil.Uint64(balance)); err != nil {
		return fmt.Errorf("%s, %d: %v", account, balance, err)
	}
	return nil
}

type MonomerClient struct {
	client *rpc.Client
	// We don't embed the ethclient.Client struct because Monomer doesn't implement the full `eth_*` interface.
	ethclient *ethclient.Client
}

func NewMonomerClient(client *rpc.Client) *MonomerClient {
	return &MonomerClient{
		client:    client,
		ethclient: ethclient.NewClient(client),
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
