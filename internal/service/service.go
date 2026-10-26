package service

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Service struct {
	Name string
	URL  *url.URL
}

func New(name, rawURL string) (*Service, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("service name cannot be empty")
	}
	if strings.TrimSpace(rawURL) == "" {
		return nil, fmt.Errorf("service %q URL cannot be empty", name)
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parse service URL: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported service URL scheme %q: expected http or https", parsedURL.Scheme)
	}
	if parsedURL.Hostname() == "" {
		return nil, fmt.Errorf("invalid service URL %q: hostname is required", rawURL)
	}
	if port := parsedURL.Port(); port != "" {
		portNumber, err := strconv.Atoi(port)
		if err != nil || portNumber < 1 || portNumber > 65535 {
			return nil, fmt.Errorf("invalid port in service URL %q: expected 1-65535", rawURL)
		}
	}
	if parsedURL.User != nil {
		return nil, fmt.Errorf("service URL must not contain embedded credentials")
	}
	if parsedURL.RawQuery != "" || parsedURL.Fragment != "" {
		return nil, fmt.Errorf("service URL must not contain a query string or fragment")
	}

	return &Service{Name: name, URL: parsedURL}, nil
}
