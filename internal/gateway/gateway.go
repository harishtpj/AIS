package gateway

import (
	"fmt"
	"context"
	"net/http"

	"github.com/harishtpj/ais/internal/proxy"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
)

type Gateway struct {
	server *http.Server
	router *router.Router
	registry *service.Registry
	proxies map[string]*proxy.Proxy
	logger zerolog.Logger
}

func New(addr string, logger zerolog.Logger, router *router.Router, registry *service.Registry, routes []router.Route) (*Gateway, error) {
	proxies := make(map[string]*proxy.Proxy, len(routes))

	for _, route := range routes {
		svc, err := registry.Get(route.Service)
		if err != nil {
			return nil, fmt.Errorf("create proxy for route %q: %w", route.Path, err)
		}

		proxies[route.Path] = proxy.New(route.Path, svc, logger)
	}

	gw := &Gateway{
		router: router,
		registry: registry,
		proxies: proxies,
		logger: logger,
	}

	gw.server = &http.Server{
		Addr: addr,
		Handler: gw,
	}

	return gw, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, err := g.router.Match(r)
	if err != nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}

	p, ok := g.proxies[route.Path]
	if !ok {
		g.logger.Error().
			Str("route", route.Path).
			Msg("proxy not found")
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	g.logger.Info().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("route", route.Path).
		Str("service", route.Service).
		Msg("request routed")
	
	p.ServeHTTP(w, r)
}

func (g *Gateway) Start() error {
	g.logger.Info().
		Str("addr", g.server.Addr).
		Msg("HTTP sever listening")
	return g.server.ListenAndServe()
}	

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info().Msg("shutting down HTTP server")
	return g.server.Shutdown(ctx)
}
