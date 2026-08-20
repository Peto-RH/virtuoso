package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestSetLogLevel(t *testing.T) {
	originalLevel := slog.SetLogLoggerLevel(slog.LevelInfo)
	t.Cleanup(func() {
		slog.SetLogLoggerLevel(originalLevel)
	})

	tests := []struct {
		name  string
		level string
		want  slog.Level
	}{
		{name: "debug", level: "debug", want: slog.LevelDebug},
		{name: "info", level: "info", want: slog.LevelInfo},
		{name: "warn", level: "warn", want: slog.LevelWarn},
		{name: "error", level: "error", want: slog.LevelError},
		{name: "numeric offset", level: "INFO+1", want: slog.LevelInfo + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := setLogLevel(tt.level); err != nil {
				t.Fatalf("setLogLevel(%q) error = %v", tt.level, err)
			}

			got := slog.SetLogLoggerLevel(slog.LevelInfo)
			if got != tt.want {
				t.Errorf("setLogLevel(%q) set level to %s, want %s", tt.level, got, tt.want)
			}
		})
	}
}

func TestSetLogLevelRejectsInvalidValue(t *testing.T) {
	originalLevel := slog.SetLogLoggerLevel(slog.LevelWarn)
	t.Cleanup(func() {
		slog.SetLogLoggerLevel(originalLevel)
	})

	err := setLogLevel("nonsense")
	if err == nil {
		t.Fatal("setLogLevel() error = nil, want invalid log level error")
	}
	if !strings.Contains(err.Error(), `invalid --log-level "nonsense"`) {
		t.Errorf("setLogLevel() error = %q, want --log-level context", err)
	}

	got := slog.SetLogLoggerLevel(slog.LevelWarn)
	if got != slog.LevelWarn {
		t.Errorf("setLogLevel() changed level to %s after invalid input, want WARN", got)
	}
}

func TestAppRejectsInvalidLogLevel(t *testing.T) {
	originalLevel := slog.SetLogLoggerLevel(slog.LevelInfo)
	t.Cleanup(func() {
		slog.SetLogLoggerLevel(originalLevel)
	})

	err := newApp().Run(context.Background(), []string{"virtuoso", "--log-level", "nonsense", "status"})
	if err == nil {
		t.Fatal("app.Run() error = nil, want invalid log level error")
	}
	if !strings.Contains(err.Error(), `invalid --log-level "nonsense"`) {
		t.Errorf("app.Run() error = %q, want --log-level context", err)
	}
}
