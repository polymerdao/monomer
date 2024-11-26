package types

import (
	sdkerrors "cosmossdk.io/errors"
)

// WrapError wraps a Cosmos-SDK error with extra message while keeping the stack trace at where this func is called.
var WrapError = sdkerrors.Wrapf

var (
	// error codes starting from 1
	registerErr                 = newErrRegistry(ModuleName, 1)
	ErrInvalidL1Txs             = registerErr("invalid L1 txs")
	ErrMintETH                  = registerErr("failed to mint ETH")
	ErrBurnETH                  = registerErr("failed to burn ETH")
	ErrInvalidSender            = registerErr("invalid sender address")
	ErrL1BlockInfo              = registerErr("l1 block info")
	ErrParams                   = registerErr("params")
	ErrProcessL1UserDepositTxs  = registerErr("failed to process L1 user deposit txs")
	ErrProcessL1SystemDepositTx = registerErr("failed to process L1 system deposit tx")
	ErrInitiateFeeWithdrawal    = registerErr("failed to initiate fee withdrawal")
	ErrUpdateParams             = registerErr("failed to updated params")
)

// register new errors without hard-coding error codes

type errRegistryFunc = func(description string) *sdkerrors.Error

func newErrRegistry(codespace string, startCode uint32) errRegistryFunc {
	currentCode := startCode

	return func(description string) *sdkerrors.Error {
		err := sdkerrors.Register(codespace, currentCode, description)
		currentCode += 1
		return err
	}
}
