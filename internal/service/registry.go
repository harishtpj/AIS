package service

import (
	"fmt"
	"sync"
)

type Registry struct {
	mutex    sync.RWMutex
	services map[string]*Service
}

func NewRegistry() *Registry {
	return &Registry{services: make(map[string]*Service)}
}

func (r *Registry) Get(name string) (*Service, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	svc, ok := r.services[name]
	if !ok {
		return nil, fmt.Errorf("no such service found: %q", name)
	}
	return svc, nil
}

func (r *Registry) Exists(name string) bool {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	_, ok := r.services[name]
	return ok
}

func (r *Registry) Register(svc *Service) error {
	if svc == nil {
		return fmt.Errorf("cannot register a nil service")
	}
	if svc.Name == "" {
		return fmt.Errorf("service name cannot be empty")
	}
	if svc.URL == nil {
		return fmt.Errorf("service %q has no URL", svc.Name)
	}

	r.mutex.Lock()
	defer r.mutex.Unlock()

	if _, ok := r.services[svc.Name]; ok {
		return fmt.Errorf("service already registered: %q", svc.Name)
	}
	r.services[svc.Name] = svc
	return nil
}
