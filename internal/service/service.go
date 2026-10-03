package service

import (
	"fmt"
	"net/url"
)

type Service struct {
	Name string
	URL *url.URL
}

func New(name, rawURL string) (*Service, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse service URL: %w", err)
	}

	if parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid service URL: %q", rawURL)
	}

	return &Service{
		Name: name,
		URL: parsedURL,
	}, nil
}
