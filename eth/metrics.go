package eth

import (
	"github.com/polymerdao/monomer/node"
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
	RecordRPCMethodCall(method string, duration time.Time)
}

type metrics struct {
	node.RPCMetrics
}

func NewMetrics(namespace string) Metrics {
	return &metrics{
		node.NewRPCMetrics(
			namespace,
			MetricsSubsystem,
			"Duration of each eth RPC method call in microseconds",
			RPCMethodDurationBucketsMicroseconds,
		),
	}
}

func (m *metrics) RecordRPCMethodCall(method string, start time.Time) {
	methodCallDuration := float64(time.Since(start).Microseconds())
	m.MethodCalls.WithLabelValues(method).Observe(methodCallDuration)
}

type noopMetrics struct{}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}

func (m *noopMetrics) RecordRPCMethodCall(_ string, _ time.Time) {}
