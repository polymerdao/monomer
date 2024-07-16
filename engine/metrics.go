package engine

import (
	rpcmetrics "github.com/polymerdao/monomer/metrics"
)

const (
	MetricsSubsystem = "engine"

	ForkchoiceUpdatedV3MethodName = "forkchoiceUpdatedV3"
	GetPayloadV3MethodName        = "getPayloadV3"
	NewPayloadV3MethodName        = "newPayloadV3"
)

var (
	RPCMethodDurationBucketsMicroseconds = []float64{1, 10, 50, 100, 500, 1000}
)

// Metrics contains metrics collected from the engine package.
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
			"Duration of each engine RPC method call in microseconds",
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
