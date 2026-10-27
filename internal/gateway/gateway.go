package gateway

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/harishtpj/ais/internal/auth"
	"github.com/harishtpj/ais/internal/proxy"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
)

const maxLoginBody = 1 << 20

type Gateway struct {
	server      *http.Server
	router      *router.Router
	registry    *service.Registry
	proxies     map[string]*proxy.Proxy
	logger      zerolog.Logger
	unifiedAuth bool
	authManager *auth.Manager
	httpClient  *http.Client
}

func New(addr string, logger zerolog.Logger, router *router.Router, registry *service.Registry, unifiedAuth bool) (*Gateway, error) {
	proxies := make(map[string]*proxy.Proxy)
	for routePath, route := range router.RoutesSnapshot() {
		svc, err := registry.Get(route.Service)
		if err != nil {
			return nil, fmt.Errorf("create proxy for route %q: %w", routePath, err)
		}
		proxies[routePath] = proxy.New(routePath, svc, logger)
	}

	gw := &Gateway{
		router: router, registry: registry, proxies: proxies, logger: logger,
		unifiedAuth: unifiedAuth,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
	if unifiedAuth {
		redisDB, err := strconv.Atoi(getEnv("REDIS_DB", "0"))
		if err != nil {
			return nil, fmt.Errorf("parse REDIS_DB: %w", err)
		}
		manager, err := auth.NewManager(
			os.Getenv("AIS_JWT_SECRET"),
			getEnv("REDIS_ADDR", "127.0.0.1:6379"),
			os.Getenv("REDIS_PASS"), redisDB,
		)
		if err != nil {
			return nil, fmt.Errorf("initialize unified auth: %w", err)
		}
		gw.authManager = manager
	}

	gw.server = &http.Server{Addr: addr, Handler: gw}
	return gw, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.unifiedAuth && strings.HasPrefix(r.URL.Path, "/auth/") {
		g.handleLogin(w, r)
		return
	}

	route, err := g.router.Match(r)
	if err != nil {
		http.Error(w, "route not found", http.StatusNotFound)
		return
	}
	p, ok := g.proxies[route.Path]
	if !ok {
		g.logger.Error().Str("route", route.Path).Msg("proxy not found")
		http.Error(w, "bad gateway", http.StatusBadGateway)
		return
	}

	if g.unifiedAuth {
		svc, err := g.registry.Get(route.Service)
		if err != nil {
			http.Error(w, "service unavailable", http.StatusBadGateway)
			return
		}
		if svc.AuthStrategy != service.AuthNone {
			forwarded, err := g.requestWithBackendCredential(r, svc)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			r = forwarded
		}
	}

	g.logger.Info().Str("method", r.Method).Str("path", r.URL.Path).
		Str("route", route.Path).Str("service", route.Service).Msg("request routed")
	p.ServeHTTP(w, r)
}

func (g *Gateway) Start() error {
	g.logger.Info().Str("addr", g.server.Addr).Msg("HTTP server listening")
	return g.server.ListenAndServe()
}

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info().Msg("shutting down HTTP server")
	if err := g.server.Shutdown(ctx); err != nil {
		return err
	}
	if g.authManager != nil {
		return g.authManager.Close()
	}
	return nil
}
