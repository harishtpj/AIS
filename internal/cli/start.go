package cli

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/harishtpj/ais/internal/discovery"
	"github.com/harishtpj/ais/internal/gateway"
	"github.com/harishtpj/ais/internal/logging"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/scripting"
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
		"ais.rb",
		"Path to the AIS configuration script",
	)

	rootCmd.AddCommand(startCmd)
}

func startGateway() error {
	logger := logging.New()
	registry := service.NewRegistry()
	router := router.New()

	rbRuntime, err := scripting.NewRuntime(registry, router)
	if err != nil {
		return fmt.Errorf("create scripting runtime: %w", err)
	}
	defer rbRuntime.Close()

	if err := rbRuntime.LoadFile(configPath); err != nil {
		return err
	}
	var provider *discovery.Podman
	if mode := strings.TrimSpace(os.Getenv("AIS_DISCOVERY")); mode != "" && mode != "off" && mode != "false" {
		if mode != "podman" {
			return fmt.Errorf("unsupported AIS_DISCOVERY mode %q (supported: podman)", mode)
		}
		provider, err = discovery.NewPodman(os.Getenv("AIS_PODMAN_SOCKET"), os.Getenv("AIS_PODMAN_HOST"), logger)
		if err != nil {
			return fmt.Errorf("initialize Podman discovery: %w", err)
		}
	}

	gtway, err := gateway.New(rbRuntime.Addr, logger, router, registry, rbRuntime.UnifiedAuth)
	if err != nil {
		return fmt.Errorf("create gateway: %w", err)
	}

	signalCtx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if provider != nil {
		interval := 5 * time.Second
		if raw := os.Getenv("AIS_DISCOVERY_INTERVAL"); raw != "" {
			if seconds, parseErr := strconv.Atoi(raw); parseErr == nil && seconds > 0 {
				interval = time.Duration(seconds) * time.Second
			} else {
				return fmt.Errorf("AIS_DISCOVERY_INTERVAL must be a positive number of seconds")
			}
		}
		go discovery.NewRunner(provider, registry, interval, logger).Run(signalCtx)
	}

	svrErr := make(chan error, 1)
	go func() {
		svrErr <- gtway.Start()
	}()

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
		5*time.Second,
	)
	defer cancel()

	if err := gtway.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info().Msg("AIS Stopped Successfully!")
	return nil
}
