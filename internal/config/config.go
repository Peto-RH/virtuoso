package config

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Source      SourceConfig
	Destination DestinationConfig
}

type SourceConfig struct {
	Type string
	URI  string
}

type DestinationConfig struct {
	OrgID  string `toml:"org_id"`
	Server string
	Port   int
	Prefix string
}

func LoadConfig(path string) (*Config, error) {
	slog.Debug("loading configuration", "path", path)

	if path == "" {
		return nil, fmt.Errorf("config file path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	slog.Debug("validating configuration")
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	slog.Debug("configuration loaded successfully")
	return &cfg, nil
}

func validateConfig(cfg *Config) error {
	if cfg.Source.Type == "" {
		return fmt.Errorf("source.type is required")
	}
	if cfg.Source.URI == "" {
		return fmt.Errorf("source.uri is required")
	}

	return nil
}
