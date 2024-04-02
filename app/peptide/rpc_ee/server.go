package rpcee

import (
	"net"

	"github.com/cometbft/cometbft/libs/log"

	"github.com/cometbft/cometbft/libs/service"
	"github.com/ethereum/go-ethereum/rpc"
)

type EERPCServer struct {
	service.BaseService

	config  *Config
	log     log.Logger
	rpcAPIs []rpc.API
	srv     *httpServer
}

// New creates a new server
func NewEeRpcServer(host string, port uint16, apis []rpc.API, log log.Logger) *EERPCServer {
	s := &EERPCServer{
		config: &Config{
			Name:                 "Execution-Engine",
			HTTPHost:             host,
			HTTPPort:             port,
			HTTPPathPrefix:       "/",
			WSPathPrefix:         "/websocket",
			BatchRequestLimit:    4,
			BatchResponseMaxSize: 1000000,
		},
		log:     log,
		rpcAPIs: apis,
		srv:     newHTTPServer(log, rpc.DefaultHTTPTimeouts),
	}

	baseService := *service.NewBaseService(log, s.ServiceName(), s)
	s.BaseService = baseService
	return s
}

func (s *EERPCServer) OnStart() error {
	if err := s.srv.setListenAddr(s.config.HTTPHost, s.config.HTTPPort); err != nil {
		return err
	}
	if err := s.srv.start(s.config, s.rpcAPIs); err != nil {
		s.log.Error("failed to serve RPC server", "err", err)
		return err
	}
	return nil
}

func (s *EERPCServer) OnStop() {
	s.srv.stop()
}

// Address returns the address the server is listening on
//
// Must be called after server.Start()
func (s *EERPCServer) Address() net.Addr {
	if s.srv == nil || s.srv.listener == nil {
		return nil
	}
	return s.srv.listener.Addr()
}

func (s *EERPCServer) ServiceName() string {
	return s.config.Name
}
