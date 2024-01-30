package peptide

import (
	"context"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net"
	"net/http"
	"time"

	tmlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/service"
	cometRpcServer "github.com/cometbft/cometbft/rpc/jsonrpc/server"
	server "github.com/polymerdao/monomer/app/node/server"
)

type Endpoint = server.Endpoint

// RPCServer is a Cosmos/CometBFT compatible server
//
// It uses CometBFT amino-compatible json encoding
type RPCServer struct {
	service.BaseService

	wsManager  *cometRpcServer.WebsocketManager
	listener   net.Listener
	mux        *http.ServeMux
	config     *cometRpcServer.Config
	endpoint   Endpoint
	httpServer *http.Server
}

var _ service.Service = (*RPCServer)(nil)

// Address returns the address the server is listening on
//
// Must be called after server.Start()
func (s *RPCServer) Address() net.Addr {
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *RPCServer) OnStart() error {
	err := s.Serve()
	if err != nil {
		s.Logger.Error("failed to serve RPC server", "err", err)
	}

	return err
}

func (s *RPCServer) OnStop() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := s.httpServer.Shutdown(ctx)
	if err != nil {
		s.Logger.Error("Error shutting down RPC server", "name", s.ServiceName(), "err", err)
	}
}

func (s *RPCServer) ServiceName() string {
	return s.BaseService.String()
}

var Config = cometRpcServer.DefaultConfig()

const websocketPath = "/websocket"

type Route = map[string]*cometRpcServer.RPCFunc

type serviceHook interface {
	// called when a websocket connection is disconnected
	OnWebsocketDisconnect(remoteAddr string, logger tmlog.Logger)
	ReportMetrics()
}

func NewRPCServer(address string, route Route, provider serviceHook, name string, logger tmlog.Logger) *RPCServer {
	endpoint := server.NewEndpoint(address)
	mux := http.NewServeMux()
	wsLogger := logger.With("protocol", websocketPath)
	wsManager := cometRpcServer.NewWebsocketManager(route, cometRpcServer.OnDisconnect(func(remoteAddr string) {
		provider.OnWebsocketDisconnect(remoteAddr, wsLogger)
	}))
	// different logger prefix for websocket
	wsManager.SetLogger(wsLogger)
	mux.HandleFunc(websocketPath, wsManager.WebsocketHandler)

	cometRpcServer.RegisterRPCFuncs(mux, route, logger)

	metricsHandler := func(w http.ResponseWriter, r *http.Request) {
		provider.ReportMetrics()
		promhttp.Handler().ServeHTTP(w, r)
	}

	mux.HandleFunc("/metrics", metricsHandler)

	config := Config

	rpcServer := &RPCServer{mux: mux, config: config, endpoint: endpoint, wsManager: wsManager}
	baseService := *service.NewBaseService(logger, name, rpcServer)
	rpcServer.BaseService = baseService
	return rpcServer
}

// NOTE: This function doesn't block
func (s *RPCServer) Serve() error {
	listener, err := cometRpcServer.Listen(s.endpoint.FullAddress(), s.config)
	if err != nil {
		panic(err)
	}
	s.listener = listener
	s.Logger.Info("Listening", "proto", s.endpoint.Protocol, "addr", listener.Addr().String(), "name", s.ServiceName())

	s.httpServer = &http.Server{
		Handler:           cometRpcServer.RecoverAndLogHandler(s.mux, s.Logger),
		ReadTimeout:       s.config.ReadTimeout,
		ReadHeaderTimeout: s.config.ReadTimeout,
		WriteTimeout:      s.config.WriteTimeout,
		MaxHeaderBytes:    s.config.MaxHeaderBytes,
	}

	go s.httpServer.Serve(listener)

	return nil
}
