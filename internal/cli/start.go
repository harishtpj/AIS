package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/harishtpj/ais/internal/config"
	"github.com/harishtpj/ais/internal/gateway"
	"github.com/harishtpj/ais/internal/logging"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/service"
	"github.com/spf13/cobra"
)

var configPath string

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the AIS gateway",
	RunE: func(cmd *cobra.Command, args []string) error {
		return startGateway()
	},
}

func init() {
	startCmd.Flags().StringVarP(
		&configPath,
		"config",
		"c",
		"configs/ais.yaml",
		"Path to the AIS configuration file",
	)

	rootCmd.AddCommand(startCmd)
}

func startGateway() error {
	logger := logging.New()
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	registry, err := buildRegistry(cfg)
	if err != nil {
		return err
	}

	routes := buildRoutes(cfg)
	requestRouter := router.New(routes)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	gtway, err := gateway.New(addr, logger, requestRouter, registry, routes)
	if err != nil {
		return fmt.Errorf("create gateway: %w", err)
	}

	svrErr := make(chan error, 1)

	go func() {
		svrErr <- gtway.Start()
	}()

	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	select {
	case err := <-svrErr:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP server failed: %w", err)
		}

	case <-signalCtx.Done():
		logger.Info().Msg("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(
		context.Background(),
		5 * time.Second,
	)
	defer cancel()

	if err := gtway.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info().Msg("AIS Stopped Successfully!")
	return nil
}

func buildRegistry(cfg *config.Config) (*service.Registry, error) {
	services := make([]*service.Service, 0, len(cfg.Services))

	for name, serviceConfig := range cfg.Services {
		svc, err := service.New(name, serviceConfig.URL)
		if err != nil {
			return nil, fmt.Errorf("create service %q: %w", name, err)
		}
		services = append(services, svc)
	}

	return service.NewRegistry(services), nil
}

func buildRoutes(cfg *config.Config) []router.Route {
	routes := make([]router.Route, 0, len(cfg.Routes))

	for _, routeConfig := range cfg.Routes {
		routes = append(routes, router.Route{
			Path: routeConfig.Path,
			Service: routeConfig.Service,
		})
	}

	return routes
}
