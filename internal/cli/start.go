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
	gtway, err := gateway.New(rbRuntime.Addr, logger, router, registry)
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
		5*time.Second,
	)
	defer cancel()

	if err := gtway.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}

	logger.Info().Msg("AIS Stopped Successfully!")
	return nil
}
