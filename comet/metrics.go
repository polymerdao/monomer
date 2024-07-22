package comet

import (
	rpcmetrics "github.com/polymerdao/monomer/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const MetricsSubsystem = "comet"

var RPCMethodDurationBucketsMicroseconds = []float64{1, 10, 50, 100, 500, 1000}

// Metrics contains metrics collected from the engine package.
type Metrics interface {
	rpcmetrics.Metrics
	RecordSubscribe(string)
	RecordUnsubscribe(string)
}

type cometMetrics struct {
	rpcmetrics.RPCMetrics
	activeSubscriptions *prometheus.GaugeVec
}

func NewMetrics(namespace string) Metrics {
	return &cometMetrics{
		RPCMetrics: rpcmetrics.NewRPCMetrics(
			namespace,
			MetricsSubsystem,
			"Duration of each comet API method call in microseconds",
			RPCMethodDurationBucketsMicroseconds,
		),
		activeSubscriptions: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "active_subscriptions",
			Help:      "Number of active comet subscriptions for a given query",
		}, []string{
			"query",
		}),
	}
}

func (m *cometMetrics) RecordSubscribe(query string) {
	m.activeSubscriptions.WithLabelValues(query).Inc()
}

func (m *cometMetrics) RecordUnsubscribe(query string) {
	m.activeSubscriptions.WithLabelValues(query).Dec()
}

type noopMetrics struct {
	rpcmetrics.RPCNoopMetrics
}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}

func (m *noopMetrics) RecordSubscribe(_ string)   {}
func (m *noopMetrics) RecordUnsubscribe(_ string) {}
