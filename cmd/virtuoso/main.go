package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
)

func main() {
	app := newApp()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Println(err.Error())
		if !slog.Default().Enabled(context.Background(), slog.LevelDebug) {
			slog.Info("an error occurred, rerun the program with --log-level debug for more details")
		}
		os.Exit(1)
	}
}

func newApp() *cli.Command {
	return &cli.Command{
		Name:    "virtuoso",
		Usage:   "report virtual guest IDs to subscription managers",
		Version: "0.1.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Value: "/etc/virtuoso/virtuoso.toml",
				Usage: "path to config `FILE`",
			},
			&cli.StringFlag{
				Name:  "log-level",
				Value: "info",
				Usage: "log `LEVEL` (debug, info, warn, error)",
			},
		},
		Before: beforeAction,
		Commands: []*cli.Command{
			statusCommand,
			runCommand,
		},
	}
}

func beforeAction(ctx context.Context, cmd *cli.Command) (context.Context, error) {
	if err := setLogLevel(cmd.String("log-level")); err != nil {
		return ctx, err
	}

	slog.Info("starting up", "pid", os.Getpid())
	return ctx, nil
}

func setLogLevel(level string) error {
	var slogLevel slog.Level
	if err := slogLevel.UnmarshalText([]byte(level)); err != nil {
		return fmt.Errorf("invalid --log-level %q: %w", level, err)
	}

	slog.SetLogLoggerLevel(slogLevel)
	return nil
}
