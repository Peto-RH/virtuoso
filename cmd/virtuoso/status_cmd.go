package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/destination/candlepin"
	"github.com/Peto-RH/virtuoso/internal/provider"
	"github.com/urfave/cli/v3"
)

const (
	status_timeout = 10 * time.Second
)

var statusCommand = &cli.Command{
	Name:   "status",
	Usage:  "validate configuration and connectivity",
	Action: statusAction,
}

func statusAction(ctx context.Context, cmd *cli.Command) error {
	slog.Info("starting validation")

	configPath := cmd.Root().String("config")
	cfg, err := config.LoadConfig(configPath, true)
	if err != nil {
		slog.Error("configuration validation failed", "err", err)
		return fmt.Errorf("config validation failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, status_timeout)
	defer cancel()

	sourceProvider, err := provider.NewProvider(ctx, &cfg.Source)
	if err != nil {
		slog.Error("failed to connect to source", "source_type", cfg.Source.Type, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}
	defer func() {
		if err := sourceProvider.Close(); err != nil {
			slog.Warn("failed to close source", "source_type", cfg.Source.Type, "err", err)
		}
	}()

	if err := sourceProvider.Ping(ctx); err != nil {
		slog.Error("source ping failed", "source_type", cfg.Source.Type, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}

	slog.Info("source validated successfully", "source_type", cfg.Source.Type)

	candlepinClient, err := candlepin.NewCandlepinClient(&cfg.Destination)
	if err != nil {
		slog.Error("failed to create Candlepin client", "err", err)
		return fmt.Errorf("destination creation failed: %w", err)
	}
	defer candlepinClient.Close()

	if err := candlepinClient.Ping(ctx); err != nil {
		slog.Error("destination ping failed", "err", err)
		return fmt.Errorf("destination validation failed: %w", err)
	}

	slog.Info("validation completed successfully")
	return nil
}
