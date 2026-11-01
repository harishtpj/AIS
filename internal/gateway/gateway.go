package gateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/harishtpj/ais/internal/auth"
	"github.com/harishtpj/ais/internal/config"
	"github.com/harishtpj/ais/internal/geolocation"
	"github.com/harishtpj/ais/internal/proxy"
	"github.com/harishtpj/ais/internal/responses"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/scripting"
	"github.com/harishtpj/ais/internal/service"
	"github.com/harishtpj/ais/internal/translation"
	"github.com/rs/zerolog"
)

const maxLoginBody = 1 << 20

type Gateway struct {
	server        *http.Server
	router        *router.Router
	registry      *service.Registry
	logger        zerolog.Logger
	unifiedAuth   bool
	authManager   *auth.Manager
	httpClient    *http.Client
	policyRuntime *scripting.Runtime
	geoResolver   *geolocation.IPAPIResolver
	translation   *translation.Manager
}

func New(addr string, logger zerolog.Logger, router *router.Router, registry *service.Registry, unifiedAuth bool, policyRuntime *scripting.Runtime, translator *translation.Manager) (*Gateway, error) {
	if router == nil {
		return nil, fmt.Errorf("router cannot be nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("service registry cannot be nil")
	}
	if err := validateListenAddress(addr); err != nil {
		return nil, err
	}

	gw := &Gateway{
		router: router, registry: registry, logger: logger,
		unifiedAuth:   unifiedAuth,
		httpClient:    &http.Client{Timeout: 10 * time.Second},
		policyRuntime: policyRuntime,
		geoResolver:   geolocation.NewIPAPIResolver(),
		translation:   translator,
	}
	if unifiedAuth {
		redisDB, err := strconv.Atoi(config.GetEnv("REDIS_DB", "0"))
		if err != nil {
			return nil, fmt.Errorf("parse REDIS_DB: %w", err)
		}
		manager, err := auth.NewManager(
			os.Getenv("AIS_JWT_SECRET"),
			config.GetEnv("REDIS_ADDR", "127.0.0.1:6379"),
			os.Getenv("REDIS_PASS"), redisDB,
		)
		if err != nil {
			return nil, fmt.Errorf("initialize unified auth: %w", err)
		}
		gw.authManager = manager
	}

	gw.server = &http.Server{
		Addr:              addr,
		Handler:           gw,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return gw, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.unifiedAuth && strings.HasPrefix(r.URL.Path, "/auth/") {
		g.handleLogin(w, r)
		return
	}

	route, err := g.router.Match(r)
	if err != nil {
		g.logger.Debug().Err(err).Str("path", r.URL.Path).Msg("route not found")
		responses.WriteError(w, http.StatusNotFound, "route_not_found", "No route matches the requested path.")
		return
	}
	baseService := route.Service
	svc, err := g.registry.Get(baseService)
	if err != nil || svc == nil || svc.URL == nil {
		g.logger.Warn().Str("service", baseService).Msg("route has no available backend")
		responses.WriteError(w, http.StatusBadGateway, "service_unavailable", "The requested service is unavailable.")
		return
	}

	if g.policyRuntime != nil {
		clientIP := requestClientIP(r)
		location := geolocation.Location{Source: "not_requested"}
		if g.policyRuntime.PoliciesNeedLocation(r.URL.Path, baseService) {
			lookupCtx, cancel := context.WithTimeout(r.Context(), 2200*time.Millisecond)
			resolved, lookupErr := g.geoResolver.Resolve(lookupCtx, net.ParseIP(clientIP))
			cancel()
			location = resolved
			if lookupErr != nil {
				g.logger.Debug().Err(lookupErr).Str("client_ip", clientIP).Msg("geolocation unavailable; policy receives an unavailable location")
			}
		}
		target, applied, policyErr := g.policyRuntime.EvaluatePolicies(r.Method, r.URL.Path, clientIP, baseService, location, time.Now())
		if policyErr != nil {
			g.logger.Error().Err(policyErr).Str("path", r.URL.Path).Msg("policy evaluation failed")
			responses.WriteError(w, http.StatusInternalServerError, "policy_evaluation_failed", "The request policy could not be evaluated.")
			return
		}
		if applied {
			route.Service = target
			svc, err = g.registry.Get(target)
			if err != nil || svc == nil || svc.URL == nil {
				g.logger.Warn().Str("service", target).Msg("policy selected an unavailable backend")
				responses.WriteError(w, http.StatusBadGateway, "service_unavailable", "The selected service is unavailable.")
				return
			}
		}
	}

	if g.unifiedAuth {
		if svc.AuthStrategy != service.AuthNone {
			forwarded, err := g.requestWithBackendCredential(r, svc)
			if err != nil {
				responses.WriteError(w, http.StatusUnauthorized, "unauthorized", "Valid gateway credentials are required.")
				return
			}
			r = forwarded
		}
	}

	backendPath := r.URL.Path
	if route.Path != "/" {
		backendPath = strings.TrimPrefix(backendPath, route.Path)
		if backendPath == "" {
			backendPath = "/"
		}
	}
	if g.translation != nil {
		body, readErr := io.ReadAll(io.LimitReader(r.Body, 10<<20))
		if readErr != nil {
			responses.WriteError(w, http.StatusBadRequest, "invalid_request_body", "The request body could not be read.")
			return
		}
		translatedBody, applied, translateErr := g.translation.Translate(route.Service, r.Method, backendPath, body, false)
		if translateErr != nil {
			responses.WriteError(w, http.StatusBadRequest, "request_translation_failed", "The request could not be translated.")
			return
		}
		if applied {
			r.Body = io.NopCloser(bytes.NewReader(translatedBody))
			r.ContentLength = int64(len(translatedBody))
			r.Header.Set("Content-Length", strconv.Itoa(len(translatedBody)))
		} else {
			r.Body = io.NopCloser(bytes.NewReader(body))
		}
	}

	g.logger.Info().Str("method", r.Method).Str("path", r.URL.Path).
		Str("route", route.Path).Str("service", route.Service).Msg("request routed")
	if g.translation == nil {
		proxy.New(route.Path, svc, g.logger).ServeHTTP(w, r)
		return
	}
	recorder := httptest.NewRecorder()
	proxy.New(route.Path, svc, g.logger).ServeHTTP(recorder, r)
	responseBody := recorder.Body.Bytes()
	if recorder.Code < http.StatusOK || recorder.Code >= http.StatusMultipleChoices {
		for name, values := range recorder.Header() {
			for _, value := range values {
				w.Header().Add(name, value)
			}
		}
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(responseBody)
		return
	}
	translatedResponse, applied, translateErr := g.translation.Translate(route.Service, r.Method, backendPath, responseBody, true)
	if translateErr != nil {
		responses.WriteError(w, http.StatusBadGateway, "response_translation_failed", "The backend response could not be translated.")
		return
	}
	for name, values := range recorder.Header() {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	if applied {
		responseBody = translatedResponse
		w.Header().Set("Content-Length", strconv.Itoa(len(responseBody)))
	}
	w.WriteHeader(recorder.Code)
	_, _ = w.Write(responseBody)
}

func validateListenAddress(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid bind address %q: %w", addr, err)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return fmt.Errorf("invalid port in bind address %q: expected 1-65535", addr)
	}
	return nil
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

func requestClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}
	if ip := net.ParseIP(strings.TrimSpace(r.RemoteAddr)); ip != nil {
		return ip.String()
	}
	return ""
}
