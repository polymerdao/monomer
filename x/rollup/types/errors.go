package types

import (
	sdkerrors "cosmossdk.io/errors"
)

// WrapError wraps an Cosmos-SDK error with extra message while keeping the stack trace at where this func is called.
var WrapError = sdkerrors.Wrapf

var (
	// error codes starting from 1
	registerErr     = newErrRegistry(ModuleName, 1)
	ErrInvalidL1Txs = registerErr("invalid L1 txs")
	ErrMintETH      = registerErr("failed to mint ETH")
	ErrL1BlockInfo  = registerErr("L1 block info")
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
