package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server ServerConfig `yaml:"server"`
	Services map[string]ServiceConfig `yaml:"services"`
	Routes []RouteConfig `yaml:"routes"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int `yaml:"port"`
}

type ServiceConfig struct {
	URL string `yaml:"url"`
}

type RouteConfig struct {
	Path string `yaml:"path"`
	Service string `yaml:"service"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}
