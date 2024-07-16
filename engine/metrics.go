package engine

import (
	rpcmetrics "github.com/polymerdao/monomer/metrics"
	"time"
)

const (
	MetricsSubsystem = "engine"

	ForkchoiceUpdatedV3MethodName = "forkchoiceUpdatedV3"
	GetPayloadV3MethodName        = "getPayload"
	NewPayloadV3MethodName        = "newPayload"
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
			"Duration of each engine RPC method call in microseconds",
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

func (m *noopMetrics) RecordRPCMethodCall(method string, start time.Time) {}
