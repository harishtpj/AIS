package proxy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httputil"

	"github.com/harishtpj/ais/internal/responses"
	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
)

type Proxy struct {
	handler http.Handler
	logger  zerolog.Logger
}

func New(routePath string, svc *service.Service, logger zerolog.Logger) *Proxy {
	rProxy := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(svc.TargetURL())
			pr.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			if errors.Is(err, context.Canceled) || r.Context().Err() == context.Canceled {
				logger.Debug().Err(err).Msg("upstream request canceled")
				return
			}

			status := http.StatusBadGateway
			code := "upstream_unavailable"
			message := "The upstream service could not be reached."
			var netErr net.Error
			if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
				status = http.StatusGatewayTimeout
				code = "upstream_timeout"
				message = "The upstream service did not respond in time."
			}

			logger.Error().Err(err).Int("status", status).Msg("reverse proxy request failed")
			responses.WriteError(w, status, code, message)
		},
	}

	var handler http.Handler = rProxy
	if routePath != "/" {
		handler = http.StripPrefix(routePath, rProxy)
	}

	return &Proxy{handler: handler, logger: logger}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.logger.Debug().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("proxying request")

	p.handler.ServeHTTP(w, r)
}
