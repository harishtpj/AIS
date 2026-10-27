package discovery

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/harishtpj/ais/internal/service"
	"github.com/rs/zerolog"
	"go.podman.io/podman/v6/pkg/bindings"
	"go.podman.io/podman/v6/pkg/bindings/pods"
)

const (
	labelDiscover = "ais/discover"
	labelService  = "ais/service"
	labelAuth     = "ais/auth-strategy"
	labelAuthPath = "ais/auth-path"
	labelPort     = "ais/port"
)

type Provider interface {
	Discover(context.Context) (map[string]*service.Service, error)
}

type Podman struct {
	socketPath string
	host       string
	logger     zerolog.Logger
}

func NewPodman(socketPath string, host string, logger zerolog.Logger) (*Podman, error) {
	if socketPath == "" {
		socketPath = os.Getenv("AIS_PODMAN_SOCKET")
	}

	if socketPath == "" {
		runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
		if runtimeDir == "" {
			return nil, fmt.Errorf("Podman socket path unavailable; set AIS_PODMAN_SOCKET")
		}
		socketPath = filepath.Join(runtimeDir, "podman", "podman.sock")
	}

	if host == "" {
		host = os.Getenv("AIS_PODMAN_HOST")
	}
	if host == "" {
		host = "127.0.0.1"
	}

	return &Podman{
		socketPath: socketPath,
		host:       host,
		logger:     logger,
	}, nil
}

func (p *Podman) Discover(ctx context.Context) (map[string]*service.Service, error) {
	socketURL := (&url.URL{Scheme: "unix", Path: p.socketPath}).String()

	connCtx, err := bindings.NewConnection(ctx, socketURL)
	if err != nil {
		return nil, fmt.Errorf("connect to Podman: %w", err)
	}

	podList, err := pods.List(connCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("list Podman pods: %w", err)
	}

	grouped := make(map[string][]*url.URL)
	settings := make(map[string]serviceSettings)

	for _, pod := range podList {
		if pod == nil {
			continue
		}

		if pod.Labels[labelDiscover] != "true" ||
			!strings.EqualFold(pod.Status, "Running") {
			continue
		}

		name := strings.TrimSpace(pod.Labels[labelService])
		if name == "" {
			p.logger.Warn().
				Str("pod", pod.Name).
				Msg("ignoring discoverable pod without ais/service label")
			continue
		}

		targetPort, err := strconv.Atoi(pod.Labels[labelPort])
		if err != nil || targetPort < 1 || targetPort > 65535 {
			p.logger.Warn().
				Str("pod", pod.Name).
				Str("service", name).
				Msg("ignoring pod with invalid ais/port label")
			continue
		}

		strategy, err := normalizeAuth(pod.Labels[labelAuth])
		if err != nil {
			p.logger.Warn().
				Str("pod", pod.Name).
				Str("service", name).
				Err(err).
				Msg("ignoring pod with invalid ais/auth-strategy label")
			continue
		}

		authPath := pod.Labels[labelAuthPath]
		if authPath == "" {
			authPath = "/login"
		}
		if !validAuthPath(authPath) {
			p.logger.Warn().
				Str("pod", pod.Name).
				Str("service", name).
				Msg("ignoring pod with invalid ais/auth-path label")
			continue
		}

		inspected, err := pods.Inspect(connCtx, pod.Name, nil)
		if err != nil {
			return nil, fmt.Errorf("inspect Podman pod %q: %w", pod.Name, err)
		}

		if inspected == nil || inspected.InfraConfig == nil {
			p.logger.Warn().
				Str("pod", pod.Name).
				Msg("ignoring pod without infra port configuration")
			continue
		}

		containerPort := strconv.Itoa(targetPort) + "/tcp"
		bindingsForPort := inspected.InfraConfig.PortBindings[containerPort]

		if len(bindingsForPort) == 0 {
			p.logger.Warn().
				Str("pod", pod.Name).
				Str("port", containerPort).
				Msg("discoverable pod has no published host port")
			continue
		}

		added := false

		for _, binding := range bindingsForPort {
			hostPort, err := strconv.Atoi(binding.HostPort)
			if err != nil || hostPort < 1 || hostPort > 65535 {
				continue
			}

			endpoint := &url.URL{
				Scheme: "http",
				Host:   net.JoinHostPort(p.host, strconv.Itoa(hostPort)),
			}

			grouped[name] = append(grouped[name], endpoint)
			added = true
		}

		if !added {
			continue
		}

		cfg := serviceSettings{
			authStrategy: strategy,
			authPath:     authPath,
		}

		if old, exists := settings[name]; exists && old != cfg {
			return nil, fmt.Errorf("pods discovered for service %q disagree on authentication labels", name)
		}

		settings[name] = cfg
	}

	result := make(map[string]*service.Service, len(grouped))

	for name, endpoints := range grouped {
		cfg := settings[name]

		svc, err := service.NewDiscovered(name, endpoints, cfg.authStrategy, cfg.authPath)
		if err != nil {
			return nil, fmt.Errorf("build discovered service %q: %w", name, err)
		}

		result[name] = svc
	}

	return result, nil
}

type serviceSettings struct {
	authStrategy string
	authPath     string
}

func validAuthPath(path string) bool {
	return strings.HasPrefix(path, "/") &&
		!strings.ContainsAny(path, "?#") &&
		!strings.Contains(path, "//")
}

func normalizeAuth(value string) (string, error) {
	switch strings.TrimSpace(value) {
	case "", "none":
		return service.AuthNone, nil
	case "session", "sessions":
		return service.AuthSessions, nil
	case "opaque-token", "opq_tok":
		return service.AuthOpaque, nil
	default:
		return "", fmt.Errorf("unsupported strategy %q", value)
	}
}
