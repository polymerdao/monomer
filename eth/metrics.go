package eth

import (
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"time"
)

const (
	MetricsSubsystem = "eth"
)

var (
	MillisecondDurationBuckets = []float64{1, 10, 50, 100, 500, 1000, 5000, 10000, 100000}
)

// Metrics contains metrics collected from the eth RPC package.
type Metrics interface {
	RecordRPCMethodCall(method string)
	RecordRPCMethodDuration(method string, duration time.Time)
}

// TODO: should there be a shared rpc metrics struct that can be reused for engine rpc metrics?
type metrics struct {
	// Count of each eth RPC method call.
	numMethodCalls *stdprometheus.CounterVec

	// Timing for each eth RPC method call.
	methodCallDuration *stdprometheus.HistogramVec
}

func NewMetrics(namespace string) Metrics {
	return &metrics{
		numMethodCalls: promauto.NewCounterVec(stdprometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "method_calls",
			Help:      "Count of each eth RPC method call.",
		}, []string{
			"method",
		}),
		methodCallDuration: promauto.NewHistogramVec(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "method_call_duration",
			Help:      "Timing for each eth RPC method call in ms.",
			Buckets:   MillisecondDurationBuckets,
		}, []string{
			"method",
		}),
	}
}

func (m *metrics) RecordRPCMethodCall(method string) {
	m.numMethodCalls.WithLabelValues(method).Add(1)
}

func (m *metrics) RecordRPCMethodDuration(method string, start time.Time) {
	m.methodCallDuration.WithLabelValues(method).Observe(float64(time.Since(start).Milliseconds()))
}

// TODO: make one universal noopMetrics?
type noopMetrics struct{}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}

func (m *noopMetrics) RecordRPCMethodCall(_ string)                  {}
func (m *noopMetrics) RecordRPCMethodDuration(_ string, _ time.Time) {}
