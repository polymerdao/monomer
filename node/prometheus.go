package node

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/polymerdao/monomer/comet"
	"github.com/polymerdao/monomer/engine"
	"github.com/polymerdao/monomer/environment"
	"github.com/polymerdao/monomer/eth"
	"github.com/polymerdao/monomer/mempool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (n *Node) startPrometheusServer(ctx context.Context, env *environment.Env) error {
	if n.prometheusCfg.IsPrometheusEnabled() {
		if err := n.prometheusCfg.ValidateBasic(); err != nil {
			return fmt.Errorf("validate prometheus instrumentation config: %v", err)
		}
		promMux := http.NewServeMux()
		promMux.Handle("/metrics", promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer, promhttp.HandlerFor(
				prometheus.DefaultGatherer,
				promhttp.HandlerOpts{MaxRequestsInFlight: n.prometheusCfg.MaxOpenConnections},
			),
		))
		promListener, err := net.Listen("tcp", n.prometheusCfg.PrometheusListenAddr)
		if err != nil {
			return fmt.Errorf("set up monomer prometheus metrics listener: %v", err)
		}
		promServer := makeHTTPService(promMux, promListener)
		env.Go(func() {
			if err := promServer.Run(ctx); err != nil {
				n.eventListener.OnPrometheusServeErr(fmt.Errorf("run prometheus metrics server: %v", err))
			}
		})
	}
	return nil
}

func (n *Node) registerMetrics() (eth.Metrics, engine.Metrics, comet.Metrics, mempool.Metrics) {
	if n.prometheusCfg.IsPrometheusEnabled() {
		namespace := n.prometheusCfg.Namespace
		return eth.NewMetrics(namespace),
			engine.NewMetrics(namespace),
			comet.NewMetrics(namespace),
			mempool.NewMetrics(namespace)
	}
	return eth.NewNoopMetrics(),
		engine.NewNoopMetrics(),
		comet.NewNoopMetrics(),
		mempool.NewNoopMetrics()
}
