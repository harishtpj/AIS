package gateway

import (
	"context"
	"log/slog"
	"net/http"
)

type Gateway struct {
	server *http.Server
	logger *slog.Logger
}

func New(addr string, logger *slog.Logger) *Gateway {
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
	g.logger.Info("HTTP sever listening", "addr", g.server.Addr)
	return g.server.ListenAndServe()
}	

func (g *Gateway) Shutdown(ctx context.Context) error {
	g.logger.Info("shutting down HTTP server")
	return g.server.Shutdown(ctx)
}
