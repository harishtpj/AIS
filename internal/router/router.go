package router

import (
	"fmt"
	"maps"
	"net/http"
	"strings"
	"sync"
)

type Router struct {
	mu     sync.RWMutex
	routes map[string]Route
}

func New() *Router {
	return &Router{routes: make(map[string]Route)}
}

func (r *Router) AddRoute(path, serviceName string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	if strings.TrimSpace(serviceName) == "" {
		return fmt.Errorf("service name cannot be empty for route %q", path)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.routes[path]; exists {
		return fmt.Errorf("route already exists: %q", path)
	}
	r.routes[path] = Route{Path: path, Service: serviceName}
	return nil
}

func (r *Router) Match(req *http.Request) (*Route, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("cannot match a nil request or URL")
	}

	path := req.URL.Path
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched *Route
	longestMatch := -1
	for _, route := range r.routes {
		if !matches(path, route.Path) {
			continue
		}
		if len(route.Path) > longestMatch {
			routeCopy := route
			matched = &routeCopy
			longestMatch = len(route.Path)
		}
	}

	if matched == nil {
		return nil, fmt.Errorf("no route found for %s %s", req.Method, path)
	}
	return matched, nil
}

func (r *Router) RoutesSnapshot() map[string]Route {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := make(map[string]Route, len(r.routes))
	maps.Copy(snapshot, r.routes)
	return snapshot
}

func validatePath(path string) error {
	if path == "" || !strings.HasPrefix(path, "/") {
		return fmt.Errorf("route path %q must start with '/'", path)
	}
	if strings.ContainsAny(path, "?#") {
		return fmt.Errorf("route path %q must not contain a query or fragment", path)
	}
	if path != "/" && strings.HasSuffix(path, "/") {
		return fmt.Errorf("route path %q must not end with '/' (except for the root route)", path)
	}
	if strings.Contains(path, "//") {
		return fmt.Errorf("route path %q must not contain repeated slashes", path)
	}
	return nil
}

func matches(path, prefix string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}
