package eth

import (
	rpcmetrics "github.com/polymerdao/monomer/metrics"
)

const (
	MetricsSubsystem = "eth"

	ChainIDMethodName          = "chainId"
	GetBlockByNumberMethodName = "getBlockByNumber"
	GetBlockByHashMethodName   = "getBlockByHash"
)

var RPCMethodDurationBucketsMicroseconds = []float64{1, 10, 50, 100, 500, 1000}

// Metrics contains metrics collected from the eth package.
type Metrics interface {
	rpcmetrics.Metrics
}

type metrics struct {
	rpcmetrics.RPCMetrics
}

func NewMetrics(namespace string) Metrics {
	return &metrics{
		rpcmetrics.NewRPCMetrics(
			namespace,
			MetricsSubsystem,
			"Duration of each eth RPC method call in microseconds",
			RPCMethodDurationBucketsMicroseconds,
		),
	}
}

type noopMetrics struct {
	rpcmetrics.RPCNoopMetrics
}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}
