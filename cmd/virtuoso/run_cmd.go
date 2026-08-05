package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/destination/candlepin"
	"github.com/Peto-RH/virtuoso/internal/provider"
	"github.com/urfave/cli/v3"
)

const (
	run_timeout = 10 * time.Minute
)

var runCommand = &cli.Command{
	Name:  "run",
	Usage: "collect and report host-guest data",
	Flags: []cli.Flag{
		&cli.BoolFlag{
			Name:  "print",
			Usage: "print report to stdout instead of sending to destination",
		},
	},
	Action: runAction,
}

func runAction(ctx context.Context, cmd *cli.Command) error {
	configPath := cmd.Root().String("config")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load configuration", "err", err)
		return err
	}

	ctx, cancel := context.WithTimeout(ctx, run_timeout)
	defer cancel()

	libvirtProvider, err := provider.NewLibvirtProvider(ctx, cfg.Source.URI)
	if err != nil {
		slog.Error("failed to connect to source", "uri", cfg.Source.URI, "err", err)
		return err
	}
	defer libvirtProvider.Close()

	slog.Info("starting data collection", "uri", cfg.Source.URI)
	hypervisor, err := libvirtProvider.Collect(ctx)
	if err != nil {
		slog.Error("data collection failed", "err", err)
		return fmt.Errorf("data collection failed: %w", err)
	}
	slog.Info("data collection completed", "guests", len(hypervisor.Guests))

	if cmd.Bool("print") {
		return candlepin.WriteHostGuestMapping(os.Stdout, hypervisor)
	}

	candlepinClient, err := candlepin.NewCandlepinClient(&cfg.Destination)
	if err != nil {
		slog.Error("failed to create Candlepin client", "err", err)
		return err
	}
	defer candlepinClient.Close()

	if err := candlepinClient.Send(ctx, hypervisor); err != nil {
		slog.Error("failed to send to Candlepin", "err", err)
		return err
	}

	slog.Info("data sent successfully")
	return nil
}
