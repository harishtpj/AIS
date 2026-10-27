package discovery

import (
	"context"
	"time"

	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
)

type Runner struct {
	provider Provider
	registry *service.Registry
	interval time.Duration
	logger   zerolog.Logger
}

func NewRunner(provider Provider, registry *service.Registry, interval time.Duration, logger zerolog.Logger) *Runner {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Runner{provider: provider, registry: registry, interval: interval, logger: logger}
}

func (r *Runner) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		r.reconcile(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) reconcile(ctx context.Context) {
	discovered, err := r.provider.Discover(ctx)
	if err != nil {
		r.logger.Error().Err(err).Msg("service discovery failed; keeping last known registry")
		return
	}
	if err := r.registry.ReplaceDiscovered(discovered); err != nil {
		r.logger.Error().Err(err).Msg("could not update discovered services")
		return
	}
	r.logger.Info().Int("services", len(discovered)).Msg("service discovery reconciled")
}
