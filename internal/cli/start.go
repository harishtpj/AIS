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

	"github.com/harishtpj/ais/internal/config"
	"github.com/harishtpj/ais/internal/discovery"
	"github.com/harishtpj/ais/internal/gateway"
	"github.com/harishtpj/ais/internal/logging"
	"github.com/harishtpj/ais/internal/ontology"
	"github.com/harishtpj/ais/internal/router"
	"github.com/harishtpj/ais/internal/scripting"
	"github.com/harishtpj/ais/internal/service"
	"github.com/harishtpj/ais/internal/slm"
	"github.com/harishtpj/ais/internal/translation"
	"github.com/spf13/cobra"
)

var (
	configPath       string
	canonicalAPIPath string
)

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
	startCmd.Flags().StringVarP(
		&canonicalAPIPath,
		"canonical",
		"A",
		"",
		"Path to the canonical entity YAML file",
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
	if err := rbRuntime.ValidatePolicies(); err != nil {
		return fmt.Errorf("validate policies: %w", err)
	}
	var ontologyStore *ontology.Store
	var translator *translation.Manager
	var document *ontology.Document
	if strings.TrimSpace(canonicalAPIPath) != "" {
		var loadErr error
		document, loadErr = ontology.LoadFile(canonicalAPIPath)
		if loadErr != nil {
			return loadErr
		}
		redisDB, parseErr := strconv.Atoi(config.GetEnv("AIS_ONTOLOGY_REDIS_DB", config.GetEnv("REDIS_DB", "0")))
		if parseErr != nil {
			return fmt.Errorf("parse ontology Redis DB: %w", parseErr)
		}
		ontologyStore, err = ontology.NewStore(
			context.Background(),
			config.GetEnv("AIS_ONTOLOGY_REDIS_ADDR", config.GetEnv("REDIS_ADDR", "127.0.0.1:6379")),
			config.GetEnv("AIS_ONTOLOGY_REDIS_PASS", os.Getenv("REDIS_PASS")),
			redisDB,
		)
		if err != nil {
			return err
		}
		defer ontologyStore.Close()
		if err := ontologyStore.SaveDocument(context.Background(), document); err != nil {
			return fmt.Errorf("persist canonical ontology: %w", err)
		}
		logger.Info().Str("path", canonicalAPIPath).Int("entities", len(document.Entities)).
			Msg("canonical ontology loaded")
	}
	if document != nil {
		var slmClient *slm.Client
		if config.GetEnv("AIS_SLM_ENABLED", "false") == "true" {
			timeout, parseErr := time.ParseDuration(config.GetEnv("AIS_SLM_TIMEOUT", "30s"))
			if parseErr != nil {
				return fmt.Errorf("parse AIS_SLM_TIMEOUT: %w", parseErr)
			}
			slmClient, err = slm.NewOllamaClient(
				config.GetEnv("AIS_SLM_URL", "http://127.0.0.1:11434"),
				config.GetEnv("AIS_SLM_MODEL", "qwen3:0.6b"),
				timeout,
			)
			if err != nil {
				return fmt.Errorf("initialize SLM client: %w", err)
			}
		}
		translator, err = translation.NewManager(document, slmClient)
		if err != nil {
			return fmt.Errorf("initialize translation manager: %w", err)
		}
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

	refreshTranslation := func(services map[string]*service.Service) {
		if translator == nil {
			return
		}
		if refreshErr := translator.Refresh(context.Background(), services); refreshErr != nil {
			logger.Warn().Err(refreshErr).Msg("could not refresh OpenAPI translation plans")
		}
	}
	if provider != nil {
		discovered, discoverErr := provider.Discover(context.Background())
		if discoverErr != nil {
			logger.Warn().Err(discoverErr).Msg("initial service discovery failed")
		} else if replaceErr := registry.ReplaceDiscovered(discovered); replaceErr != nil {
			return fmt.Errorf("register discovered services: %w", replaceErr)
		} else {
			refreshTranslation(registry.Snapshot())
		}
	} else {
		refreshTranslation(registry.Snapshot())
	}

	gtway, err := gateway.New(rbRuntime.Addr, logger, router, registry, rbRuntime.UnifiedAuth, rbRuntime, translator)
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
		go discovery.NewRunner(provider, registry, interval, logger, refreshTranslation).Run(signalCtx)
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
