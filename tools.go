//go:build tools
// +build tools

package tools

import (
	_ "github.com/bufbuild/buf/cmd/buf"
	_ "github.com/cosmos/gogoproto/protoc-gen-gocosmos"
	_ "github.com/ethereum/go-ethereum/cmd/abigen"
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/vladopajic/go-test-coverage/v2"
	_ "go.uber.org/mock/mockgen"
	_ "mvdan.cc/gofumpt"
)
