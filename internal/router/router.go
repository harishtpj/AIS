package router

import (
	"fmt"
	"net/http"
	"strings"
)

type Router struct {
	routes []Route
}

func New(routes []Route) *Router {
	return &Router{routes: routes}
}

func (r *Router) Match(req *http.Request) (*Route, error) {
	var matched *Route
	longestMatch := 0

	for i := range r.routes {
		route := &r.routes[i]

		if !matches(req.URL.Path, route.Path) {
			continue
		}

		if len(route.Path) > longestMatch {
			matched = route
			longestMatch = len(route.Path)
		}
	}

	if matched == nil {
		return nil, fmt.Errorf("no route found for %s %s", req.Method, req.URL.Path)
	}

	return matched, nil
}

func matches(path, prefix string) bool {
	return prefix == "/" || path == prefix || strings.HasPrefix(path, prefix + "/")
}
