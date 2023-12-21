package abci_grpc

import (
	"net"
	"os"

	"google.golang.org/grpc"

	"github.com/cometbft/cometbft/abci/types"
	tmlog "github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/libs/service"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/server/grpc/gogoreflection"
	reflection "github.com/cosmos/cosmos-sdk/server/grpc/reflection/v2alpha1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	server "github.com/polymerdao/monomer/app/node/server"
	"github.com/polymerdao/monomer/app/peptide"
)

type Endpoint = server.Endpoint

type GRPCServer struct {
	service.BaseService

	endpoint Endpoint
	listener net.Listener
	server   *grpc.Server

	name string
	// callback to register services before server.Serve()
	registerService registerFunc
	opts            []grpc.ServerOption
}

type registerFunc func(*grpc.Server) error

// NewGRPCServer returns a new gRPC ABCI server
func NewGRPCServer(protoAddr, name string, registerService registerFunc, logger tmlog.Logger, opts ...grpc.ServerOption) service.Service {
	endpoint := server.NewEndpoint(protoAddr)
	s := &GRPCServer{
		endpoint:        endpoint,
		name:            name,
		registerService: registerService,
		opts:            opts,
	}
	s.BaseService = *service.NewBaseService(logger, name, s)
	return s
}

func NewAbciGRPCServer(protoAddr string, app types.ABCIApplicationServer) service.Service {
	var register = func(s *grpc.Server) error {
		types.RegisterABCIApplicationServer(s, app)
		return nil
	}

	return NewGRPCServer(protoAddr, "ABCIApplicationServer", register, tmlog.NewTMJSONLogger(tmlog.NewSyncWriter(os.Stdout)))
}

func NewChainAppGRPCServer(protoAddr string, app *peptide.PeptideApp) service.Service {
	var register = func(s *grpc.Server) error {
		// Register reflection service on gRPC server.
		app.RegisterGRPCServer(s)
		// Reflection allows consumers to build dynamic clients that can write to any
		// Cosmos SDK application without relying on application packages at compile
		// time.
		err := reflection.Register(s, reflection.Config{
			ChainID:           app.ChainId,
			SdkConfig:         sdk.GetConfig(),
			InterfaceRegistry: app.InterfaceRegistry(),
		})
		if err != nil {
			return err
		}
		// Reflection allows external clients to see what services and methods
		// the gRPC server exposes.
		gogoreflection.Register(s)
		return nil
	}

	return NewGRPCServer(
		protoAddr, "ChainAppServer", register, server.DefaultLogger(),
		grpc.ForceServerCodec(codec.NewProtoCodec(app.InterfaceRegistry()).GRPCCodec()),
	)
}

func (s *GRPCServer) OnStart() error {
	ln, err := net.Listen(s.endpoint.Protocol, s.endpoint.Host)
	if err != nil {
		return err
	}

	s.listener = ln
	s.server = grpc.NewServer(s.opts...)

	err = s.registerService(s.server)
	if err != nil {
		return err
	}

	s.Logger.Info("Listening", "proto", s.endpoint.Protocol, "addr", s.endpoint.Host, "name", s.name)
	go func() {
		if err := s.server.Serve(s.listener); err != nil {
			s.Logger.Error("Error serving gRPC server", "err", err, "name", s.name)
		}
	}()
	return nil
}

// OnStop stops the gRPC server.
func (s *GRPCServer) OnStop() {
	s.server.Stop()
}
