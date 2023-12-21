package server

import (
	"fmt"
	"net"
	"os"
	"strconv"

	tmlog "github.com/cometbft/cometbft/libs/log"
	cmtnet "github.com/cometbft/cometbft/libs/net"
	"github.com/cometbft/cometbft/libs/service"
	"github.com/samber/lo"
)

type Logger = tmlog.Logger

var DefaultLogger = func() Logger {
	return tmlog.NewTMLogger(tmlog.NewSyncWriter(os.Stdout))
}

type Endpoint struct {
	Host     string // host:port
	Protocol string // tcp, unix, etc
}

func NewEndpoint(address string) Endpoint {
	protocol, host := cmtnet.ProtocolAndAddress(address)
	return Endpoint{
		Host:     host,
		Protocol: protocol,
	}
}

func (e *Endpoint) Port() int64 {
	_, port, err := net.SplitHostPort(e.Host)
	if err != nil {
		panic(err)
	}
	return lo.Must(strconv.ParseInt(port, 10, 32))
}

func (e *Endpoint) FullAddress(proto ...string) string {
	protocol := e.Protocol
	if len(proto) > 0 {
		protocol = proto[0]
	}
	return fmt.Sprintf("%s://%s", protocol, e.Host)
}

func (e *Endpoint) IsValid() bool {
	return e.Host != "" && e.Host != "-"
}

type CompositeService struct {
	service.BaseService
	services        []service.Service
	startedServices []service.Service
}

var _ service.Service = (*CompositeService)(nil)

// NewCompositeService creates a new CompositeService with the given services.
func NewCompositeService(services ...service.Service) *CompositeService {
	cs := &CompositeService{
		services: services,
	}
	cs.BaseService = *service.NewBaseService(DefaultLogger(), "CompositeService", cs)
	return cs
}

// try to start all services, if any fails, stop all already started services and return the error
func (s *CompositeService) OnStart() error {
	for _, service := range s.services {
		err := service.Start()
		if err != nil {
			s.OnStop()
			return err
		} else {
			s.startedServices = append(s.startedServices, service)
		}
	}
	return nil
}

// stop started services
func (s *CompositeService) OnStop() {
	for _, service := range s.startedServices {
		err := service.Stop()
		if err != nil {
			s.Logger.Error("Error stopping service", "name", service.String(), "err", err)
		}
	}
}
