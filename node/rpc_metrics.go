package node

import (
	stdprometheus "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TODO: should this belong in a separate metrics package?
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
