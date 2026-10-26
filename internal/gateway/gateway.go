package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/harishtpj/ais/internal/proxy"
	"github.com/harishtpj/ais/internal/responses"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
)

type Gateway struct {
	server  *http.Server
	router  *router.Router
	proxies map[string]*proxy.Proxy
	logger  zerolog.Logger
}

func New(addr string, logger zerolog.Logger, routes *router.Router, registry *service.Registry) (*Gateway, error) {
	if routes == nil {
		return nil, fmt.Errorf("router cannot be nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("service registry cannot be nil")
	}
	if err := validateListenAddress(addr); err != nil {
		return nil, err
	}

	proxies := make(map[string]*proxy.Proxy)
	for path, route := range routes.RoutesSnapshot() {
		svc, err := registry.Get(route.Service)
		if err != nil {
			return nil, fmt.Errorf("create proxy for route %q: %w", path, err)
		}
		proxies[path] = proxy.New(path, svc, logger)
	}

	gw := &Gateway{router: routes, proxies: proxies, logger: logger}
	gw.server = &http.Server{
		Addr:              addr,
		Handler:           gw,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return gw, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	route, err := g.router.Match(r)
	if err != nil {
		g.logger.Debug().Err(err).Str("path", r.URL.Path).Msg("route not found")
		writeError(w, http.StatusNotFound, "route_not_found", "No route matches the requested path.")
		return
	}

	p, ok := g.proxies[route.Path]
	if !ok {
		g.logger.Error().Str("route", route.Path).Msg("proxy missing for matched route")
		writeError(w, http.StatusInternalServerError, "gateway_configuration_error", "The gateway could not handle this route.")
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
	g.logger.Info().Str("addr", g.server.Addr).Msg("HTTP server listening")
	return g.server.ListenAndServe()
}

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info().Msg("shutting down HTTP server")
	return g.server.Shutdown(ctx)
}

func validateListenAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid bind address %q: %w", addr, err)
	}
	_ = host
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("invalid port in bind address %q: expected 1-65535", addr)
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(responses.Error{Error: code, Message: message})
}
