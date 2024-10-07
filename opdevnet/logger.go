package opdevnet

import "github.com/ethereum/go-ethereum/log"

func newLogger(logger log.Logger, name string) log.Logger {
	return logger.With("opdevnet-component", name)
}
