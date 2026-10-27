package service

import (
	"fmt"
	"net/url"
	"strings"
	"sync/atomic"
)

const (
	AuthNone     = ""
	AuthSessions = "sessions"
	AuthOpaque   = "opq_tok"
)

type Service struct {
	Name         string
	URL          *url.URL
	Instances    []*url.URL
	AuthStrategy string
	AuthPath     string
	Dynamic      bool
	next         atomic.Uint64
}

func New(name, rawURL, authStrategy string) (*Service, error) {
	return NewWithOptions(name, rawURL, authStrategy, "/login")
}

func NewWithOptions(name, rawURL, authStrategy, authPath string) (*Service, error) {
	if authStrategy == "none" {
		authStrategy = AuthNone
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("service name cannot be empty")
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse service URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("invalid service URL: %q", rawURL)
	}
	if err := validateAuthStrategy(authStrategy); err != nil {
		return nil, err
	}
	if authPath == "" {
		authPath = "/login"
	}
	if !strings.HasPrefix(authPath, "/") || strings.ContainsAny(authPath, "?#") || strings.Contains(authPath, "//") {
		return nil, fmt.Errorf("auth path %q must be an absolute path without query or fragment", authPath)
	}
	return &Service{Name: name, URL: parsedURL, AuthStrategy: authStrategy, AuthPath: authPath}, nil
}

func NewDiscovered(name string, endpoints []*url.URL, authStrategy, authPath string) (*Service, error) {
	if strings.TrimSpace(name) == "" || len(endpoints) == 0 {
		return nil, fmt.Errorf("discovered service requires a name and at least one endpoint")
	}
	if err := validateAuthStrategy(authStrategy); err != nil {
		return nil, err
	}
	if authPath == "" {
		authPath = "/login"
	}
	if !strings.HasPrefix(authPath, "/") || strings.ContainsAny(authPath, "?#") || strings.Contains(authPath, "//") {
		return nil, fmt.Errorf("auth path %q must be an absolute path without query or fragment", authPath)
	}
	copyEndpoints := append([]*url.URL(nil), endpoints...)
	for _, endpoint := range copyEndpoints {
		if endpoint == nil || endpoint.Host == "" || (endpoint.Scheme != "http" && endpoint.Scheme != "https") {
			return nil, fmt.Errorf("discovered service %q has an invalid endpoint", name)
		}
	}
	return &Service{Name: name, URL: copyEndpoints[0], Instances: copyEndpoints, AuthStrategy: authStrategy, AuthPath: authPath, Dynamic: true}, nil
}

func validateAuthStrategy(strategy string) error {
	if strategy != AuthNone && strategy != AuthSessions && strategy != AuthOpaque {
		return fmt.Errorf("unsupported auth strategy %q (use :sessions or :opq_tok)", strategy)
	}
	return nil
}

func (s *Service) TargetURL() *url.URL {
	if s == nil {
		return nil
	}
	if len(s.Instances) == 0 {
		return s.URL
	}
	i := s.next.Add(1) - 1
	return s.Instances[i%uint64(len(s.Instances))]
}
