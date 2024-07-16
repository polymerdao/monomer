package eth

import (
	rpcmetrics "github.com/polymerdao/monomer/metrics"
	"time"
)

const (
	MetricsSubsystem = "eth"

	ChainIdMethodName          = "chainId"
	GetBlockByNumberMethodName = "getBlockByNumber"
	GetBlockByHashMethodName   = "getBlockByHash"
)

var (
	RPCMethodDurationBucketsMicroseconds = []float64{1, 10, 50, 100, 500, 1000, 5000, 10000, 100000}
)

// Metrics contains metrics collected from the eth package.
type Metrics interface {
	RecordRPCMethodCall(method string, start time.Time)
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

type noopMetrics struct{}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}

func (m *noopMetrics) RecordRPCMethodCall(method string, start time.Time) {}
