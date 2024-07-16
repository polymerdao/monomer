package e2e

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/state"
)

// helper shim to allow decoding of hex nonces
type auxDump struct {
	Root     string                    `json:"root"`
	Accounts map[string]auxDumpAccount `json:"accounts"`
	// Next can be set to represent that this dump is only partial, and Next
	// is where an iterator should be positioned in order to continue the dump.
	Next []byte `json:"next,omitempty"` // nil if no more accounts
}

type auxDumpAccount struct {
	Balance     string                 `json:"balance"`
	Nonce       string                 `json:"nonce"`
	Root        hexutil.Bytes          `json:"root"`
	CodeHash    hexutil.Bytes          `json:"codeHash"`
	Code        hexutil.Bytes          `json:"code,omitempty"`
	Storage     map[common.Hash]string `json:"storage,omitempty"`
	Address     *common.Address        `json:"address,omitempty"` // Address only present in iterative (line-by-line) mode
	AddressHash hexutil.Bytes          `json:"key,omitempty"`     // If we don't have address, we can output the key
}

func (d *auxDump) ToStateDump() (*state.Dump, error) {
	accounts := make(map[string]state.DumpAccount)

	for k, v := range d.Accounts { //nolint:gocritic
		nonce, err := hexutil.DecodeUint64(v.Nonce)
		if err != nil {
			return nil, fmt.Errorf("decode nonce: %v", err)
		}

		accounts[k] = state.DumpAccount{
			Balance:  v.Balance,
			Nonce:    nonce,
			Root:     v.Root,
			CodeHash: v.CodeHash,
			Code:     v.Code,
			Storage:  v.Storage,
		}
	}

	return &state.Dump{
		Root:     d.Root,
		Accounts: accounts,
		Next:     d.Next,
	}, nil
}
