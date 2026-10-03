package service

import (
	"fmt"
	"sync"
)

type Registry struct {
	mutex    sync.RWMutex
	services map[string]*Service
}

func NewRegistry(services []*Service) *Registry {
	registry := &Registry{
		services: make(map[string]*Service),
	}

	for _, service := range services {
		registry.services[service.Name] = service
	}

	return registry
}

func (r *Registry) Get(name string) (*Service, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	service, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("no such service found: %q", name)
	}

	return service, nil
}
