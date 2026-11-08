package config

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Source      SourceConfig
	Destination DestinationConfig
}

type SourceConfig struct {
	Type     string
	URI      string
	Username string
	CAFile   string `toml:"ca_file"`
}

type DestinationConfig struct {
	OrgID  string `toml:"org_id"`
	Server string
	Port   int
	Prefix string
}

func LoadConfig(path string, includeDestination bool) (*Config, error) {
	slog.Debug("loading configuration", "path", path)

	if path == "" {
		return nil, fmt.Errorf("config file path is required")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}

	var cfg Config
	var target any = &cfg
	if !includeDestination {
		target = &struct{ Source *SourceConfig }{Source: &cfg.Source}
	}
	if err := toml.Unmarshal(data, target); err != nil {
		return nil, fmt.Errorf("cannot parse config file: %w", err)
	}

	slog.Debug("validating source configuration")
	if err := validateSourceConfig(&cfg.Source); err != nil {
		return nil, err
	}
	if includeDestination {
		slog.Debug("validating destination configuration")
		if err := validateDestinationConfig(&cfg.Destination); err != nil {
			return nil, err
		}
	}

	slog.Debug("configuration loaded successfully")
	return &cfg, nil
}

func validateSourceConfig(cfg *SourceConfig) error {
	if strings.TrimSpace(cfg.Type) == "" {
		return fmt.Errorf("source.type is required")
	}
	switch cfg.Type {
	case "libvirt", "vmware":
	default:
		return fmt.Errorf("unsupported source.type %q: use libvirt or vmware", cfg.Type)
	}
	if strings.TrimSpace(cfg.URI) == "" {
		return fmt.Errorf("source.uri is required")
	}

	return nil
}

func validateDestinationConfig(cfg *DestinationConfig) error {
	if strings.TrimSpace(cfg.OrgID) == "" {
		return fmt.Errorf("destination.org_id is required")
	}
	if strings.TrimSpace(cfg.Server) == "" {
		return fmt.Errorf("destination.server is required")
	}
	server, err := url.Parse("https://" + cfg.Server)
	if err != nil || server.Host != cfg.Server || server.Hostname() == "" {
		return fmt.Errorf("destination.server must be a hostname or IP address without a scheme, credentials, path, or port")
	}
	hostname := server.Hostname()
	if cfg.Server != hostname && (cfg.Server != "["+hostname+"]" || net.ParseIP(hostname) == nil) {
		return fmt.Errorf("destination.server must be a hostname or IP address without a scheme, credentials, path, or port")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return fmt.Errorf("destination.port must be between 1 and 65535")
	}
	if !strings.HasPrefix(cfg.Prefix, "/") {
		return fmt.Errorf("destination.prefix is required and must start with '/'")
	}
	return nil
}
