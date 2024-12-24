package e2e

import (
    "context"
    "math/big"

    "github.com/ethereum/go-ethereum/ethclient"
    "github.com/ethereum/go-ethereum/core/types"
)

// Stack represents the configuration for testing
type Stack struct {
    // ... existing fields ...
    l1Client    *ethclient.Client
    l2Client    *ethclient.Client
}

// WaitL1 waits for the specified number of L1 blocks
func (s *Stack) WaitL1(numBlocks uint64) error {
    return wait(s.l1Client, numBlocks)
}

// WaitL2 waits for the specified number of L2 blocks
func (s *Stack) WaitL2(numBlocks uint64) error {
    return wait(s.l2Client, numBlocks)
}
