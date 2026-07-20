package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/Peto-RH/virtuoso/internal/config"
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
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("configuration validation failed", "err", err)
		return fmt.Errorf("config validation failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, status_timeout)
	defer cancel()

	libvirtProvider, err := provider.NewLibvirtProvider(ctx, cfg.Source.URI)
	if err != nil {
		slog.Error("failed to connect to source", "uri", cfg.Source.URI, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}
	defer libvirtProvider.Close()

	if err := libvirtProvider.Ping(ctx); err != nil {
		slog.Error("source ping failed", "uri", cfg.Source.URI, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}

	slog.Info("source validated successfully", "uri", cfg.Source.URI)

	dest, err := createDestination(&cfg.Destination)
	if err != nil {
		slog.Error("failed to create destination", "err", err)
		return fmt.Errorf("destination creation failed: %w", err)
	}
	defer dest.Close()

	if err := dest.Ping(ctx); err != nil {
		slog.Error("destination ping failed", "err", err)
		return fmt.Errorf("destination validation failed: %w", err)
	}

	slog.Info("validation completed successfully")
	return nil
}
