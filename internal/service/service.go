package service

import (
	"fmt"
	"net/url"
)

const (
	AuthNone     = ""
	AuthSessions = "sessions"
	AuthOpaque   = "opq_tok"
)

type Service struct {
	Name         string
	URL          *url.URL
	AuthStrategy string
}

func New(name, rawURL, authStrategy string) (*Service, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse service URL: %w", err)
	}
	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid service URL: %q", rawURL)
	}
	if authStrategy != AuthNone && authStrategy != AuthSessions && authStrategy != AuthOpaque {
		return nil, fmt.Errorf("unsupported auth strategy %q (use :sessions or :opq_tok)", authStrategy)
	}

	return &Service{Name: name, URL: parsedURL, AuthStrategy: authStrategy}, nil
}
