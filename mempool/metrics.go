package mempool

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const MetricsSubsystem = "mempool"

type Metrics interface {
	RecordEnqueueMempoolTx()
	RecordDequeueMempoolTx()
}

type mempoolMetrics struct {
	// Number of transactions in the mempool.
	mempoolSize prometheus.Gauge
}

func NewMetrics(namespace string) Metrics {
	return &mempoolMetrics{
		mempoolSize: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: MetricsSubsystem,
			Name:      "size",
			Help:      "Number of transactions in the mempool",
		}),
	}
}

func (m *mempoolMetrics) RecordEnqueueMempoolTx() {
	m.mempoolSize.Inc()
}

func (m *mempoolMetrics) RecordDequeueMempoolTx() {
	m.mempoolSize.Dec()
}

type noopMetrics struct{}

func NewNoopMetrics() Metrics {
	return &noopMetrics{}
}

func (m *noopMetrics) RecordEnqueueMempoolTx() {}
func (m *noopMetrics) RecordDequeueMempoolTx() {}
