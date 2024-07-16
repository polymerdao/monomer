package metrics

import (
	"time"

	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

type Metrics interface {
	RecordRPCMethodCall(method string, start time.Time)
}

type RPCMetrics struct {
	// Count and duration of each RPC method call.
	MethodCalls *stdprometheus.HistogramVec
}

func NewRPCMetrics(namespace, subsystem, info string, buckets []float64) RPCMetrics {
	return RPCMetrics{
		MethodCalls: promauto.NewHistogramVec(stdprometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "method_call",
			Help:      info,
			Buckets:   buckets,
		}, []string{
			"method",
		}),
	}
}

func (m *RPCMetrics) RecordRPCMethodCall(method string, start time.Time) {
	methodCallDuration := float64(time.Since(start).Microseconds())
	m.MethodCalls.WithLabelValues(method).Observe(methodCallDuration)
}

type RPCNoopMetrics struct{}

func (m *RPCNoopMetrics) RecordRPCMethodCall(_ string, _ time.Time) {}
