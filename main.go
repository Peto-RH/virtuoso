package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/pschrimp/virtuoso/internal/config"
	"github.com/pschrimp/virtuoso/internal/provider"
	"github.com/pschrimp/virtuoso/internal/reporter"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("%v\n", err.Error())
		if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			slog.Info("an error occurred, rerun the program with --log-level debug for more details")
		}
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "/etc/virtuoso/virtuoso.toml", "path to config file")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	// Configure log level
	if err := setLogLevel(*logLevel); err != nil {
		return err
	}

	slog.Info("starting up", "pid", os.Getpid())

	command := flag.Arg(0)
	if command == "" {
		return fmt.Errorf("no command specified (available: status, run)")
	}

	switch command {
	case "status":
		return runStatus(*configPath)
	case "run":
		return runCommand(*configPath)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func setLogLevel(level string) error {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		return fmt.Errorf("invalid log level: %s (valid: debug, info, warn, error)", level)
	}

	slog.SetLogLoggerLevel(slogLevel)
	return nil
}

func runStatus(configPath string) error {
	slog.Debug("validating configuration")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("configuration validation failed", "err", err)
		return fmt.Errorf("config validation failed: %w", err)
	}

	fmt.Println("Configuration valid")

	// Test libvirt connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	libvirtProvider, err := provider.NewLibvirtProvider(ctx, cfg.Source.URI)
	if err != nil {
		slog.Error("failed to connect to source", "uri", cfg.Source.URI, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}
	defer libvirtProvider.Close()

	// Test connection without collecting data
	err = libvirtProvider.Ping(ctx)
	if err != nil {
		slog.Error("source ping failed", "uri", cfg.Source.URI, "err", err)
		return fmt.Errorf("source validation failed: %w", err)
	}

	slog.Info("source validated successfully", "uri", cfg.Source.URI)
	fmt.Println("Source reachable")
	return nil
}

func runCommand(configPath string) error {
	// Parse run-specific flags
	runFlags := flag.NewFlagSet("run", flag.ExitOnError)
	printFlag := runFlags.Bool("print", false, "print report to stdout")
	runFlags.Parse(flag.Args()[1:])

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		slog.Error("failed to load configuration", "err", err)
		return err
	}

	ctx := context.Background()
	libvirtProvider, err := provider.NewLibvirtProvider(ctx, cfg.Source.URI)
	if err != nil {
		slog.Error("failed to connect to source", "uri", cfg.Source.URI, "err", err)
		return err
	}
	defer libvirtProvider.Close()

	slog.Info("starting data collection", "uri", cfg.Source.URI)
	rep, err := libvirtProvider.Collect(ctx)
	if err != nil {
		slog.Error("data collection failed", "err", err)
		return fmt.Errorf("data collection failed: %w", err)
	}
	slog.Info("data collection completed", "guests", len(rep.Guests))

	if *printFlag {
		jsonReporter := reporter.NewJSONReporter(os.Stdout)
		return jsonReporter.Write(rep)
	}

	return nil
}
