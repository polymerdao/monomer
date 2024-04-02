package main

import (
	"fmt"
)

type Service interface {
	Start() error
	Stop() error
}

type namedService struct {
	name    string
	service Service
}

type nodeService []namedService

func newNodeService(engineServer, ethServer, eventBus Service) nodeService {
	return []namedService{
		{
			name:    "event bus",
			service: eventBus,
		},
		{
			name:    "eth server",
			service: ethServer,
		},
		{
			name:    "engine server",
			service: engineServer,
		},
	}
}

func (n nodeService) Start() error {
	for _, namedService := range n {
		if err := namedService.service.Start(); err != nil {
			return fmt.Errorf("start %s: %v", namedService.name, err)
		}
	}
	return nil
}

func (n nodeService) Stop() error {
	for i := 0; i < len(n); i++ {
		namedService := n[len(n)-1-i] // Stop services in reverse order.
		if err := namedService.service.Stop(); err != nil {
			return fmt.Errorf("stop %s: %v", namedService.name, err)
		}
	}
	return nil
}
