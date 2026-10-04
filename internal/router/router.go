package router

import (
	"fmt"
	"net/http"
	"strings"
)

type Router struct {
	Routes map[string]Route
}

func New() *Router {
	return &Router{Routes: make(map[string]Route)}
}

func (r *Router) AddRoute(path, serviceName string) error {
	if _, exists := r.Routes[path]; exists {
		return fmt.Errorf("route already exists: %q", path)
	}

	r.Routes[path] = Route{path, serviceName}
	return nil
}

func (r *Router) Match(req *http.Request) (*Route, error) {
	var matched *Route
	longestMatch := 0

	for _, route := range r.Routes {
		if !matches(req.URL.Path, route.Path) {
			continue
		}

		if len(route.Path) > longestMatch {
			matched = &route
			longestMatch = len(route.Path)
		}
	}

	if matched == nil {
		return nil, fmt.Errorf("no route found for %s %s", req.Method, req.URL.Path)
	}

	return matched, nil
}

func matches(path, prefix string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/")
}
