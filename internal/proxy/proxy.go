package proxy

import (
	"net/http"
	"net/http/httputil"

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
			pr.SetURL(svc.URL)
			pr.SetXForwarded()
		},
	}
	handler := http.StripPrefix(routePath, rProxy)
	return &Proxy{
		handler: handler,
		logger:  logger,
	}
}

func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.logger.Debug().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Msg("proxying request")

	p.handler.ServeHTTP(w, r)
}
