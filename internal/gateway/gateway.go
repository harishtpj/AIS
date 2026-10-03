package gateway

import (
	"context"
	"net/http"

	"github.com/rs/zerolog"
)

type Gateway struct {
	server *http.Server
	logger zerolog.Logger
}

func New(addr string, logger zerolog.Logger) *Gateway {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("AIS Gateway"))
	})

	return &Gateway{
		server: &http.Server{
			Addr: addr,
			Handler: mux,
		},
		logger: logger,
	}
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
