package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name               string
		contents           string
		includeDestination bool
		want               Config
	}{
		{
			name: "libvirt without destination defaults",
			contents: `[source]
type = "libvirt"
uri = "qemu:///system"
`,
			want: Config{Source: SourceConfig{Type: "libvirt", URI: "qemu:///system"}},
		},
		{
			name:               "VMware with destination",
			includeDestination: true,
			contents: `[source]
type = "vmware"
uri = "https://vcenter.example.com/sdk"
username = " reporter "
ca_file = "/etc/virtuoso/vcenter-ca.pem"
[destination]
org_id = "example-org"
server = "satellite.example.com"
port = 8443
prefix = "/candlepin"
`,
			want: Config{
				Source: SourceConfig{
					Type: "vmware", URI: "https://vcenter.example.com/sdk",
					Username: " reporter ", CAFile: "/etc/virtuoso/vcenter-ca.pem",
				},
				Destination: DestinationConfig{
					OrgID: "example-org", Server: "satellite.example.com", Port: 8443, Prefix: "/candlepin",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.contents)
			got, err := LoadConfig(path, tt.includeDestination)
			if err != nil {
				t.Fatalf("LoadConfig() error = %v", err)
			}
			if *got != tt.want {
				t.Errorf("LoadConfig() = %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestLoadConfigRequiresDestination(t *testing.T) {
	path := writeConfig(t, "[source]\ntype = 'libvirt'\nuri = 'qemu:///system'\n")
	cfg, err := LoadConfig(path, true)
	if err == nil || !strings.Contains(err.Error(), "destination.org_id is required") {
		t.Fatalf("LoadConfig() error = %v, want missing destination error", err)
	}
	if cfg != nil {
		t.Error("LoadConfig() returned a configuration on error")
	}
}

func TestLoadConfigRejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name      string
		contents  string
		wantError string
	}{
		{name: "missing type", contents: "[source]\nuri = 'qemu:///system'", wantError: "source.type is required"},
		{name: "blank type", contents: "[source]\ntype = '   '", wantError: "source.type is required"},
		{name: "unsupported type", contents: "[source]\ntype = 'esx'\nuri = 'https://vcenter.example.com/sdk'", wantError: `unsupported source.type "esx"`},
		{name: "missing URI", contents: "[source]\ntype = 'libvirt'", wantError: "source.uri is required"},
		{name: "blank URI", contents: "[source]\ntype = 'libvirt'\nuri = \" \\t\\n\"", wantError: "source.uri is required"},
		{name: "malformed TOML", contents: "[source", wantError: "cannot parse config file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.contents)
			for _, includeDestination := range []bool{false, true} {
				cfg, err := LoadConfig(path, includeDestination)
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("LoadConfig(includeDestination=%t) error = %v, want %q", includeDestination, err, tt.wantError)
				}
				if cfg != nil {
					t.Error("LoadConfig() returned a configuration on error")
				}
			}
		})
	}
}

func TestLoadConfigFileErrors(t *testing.T) {
	t.Run("empty path", func(t *testing.T) {
		cfg, err := LoadConfig("", true)
		if err == nil || !strings.Contains(err.Error(), "config file path is required") {
			t.Fatalf("LoadConfig() error = %v, want missing path error", err)
		}
		if cfg != nil {
			t.Error("LoadConfig() returned a configuration on error")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		cfg, err := LoadConfig(filepath.Join(t.TempDir(), "missing.toml"), true)
		if !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "cannot read config file") {
			t.Fatalf("LoadConfig() error = %v, want wrapped file-not-found error", err)
		}
		if cfg != nil {
			t.Error("LoadConfig() returned a configuration on error")
		}
	})
}

func TestLoadConfigSkipsDestinationFields(t *testing.T) {
	path := writeConfig(t, `[source]
type = "libvirt"
uri = "qemu:///system"
[destination]
org_id = 123
server = ["satellite.example.com"]
port = "invalid"
prefix = false
`)
	cfg, err := LoadConfig(path, false)
	if err != nil {
		t.Fatalf("LoadConfig(includeDestination=false) error = %v", err)
	}
	want := Config{Source: SourceConfig{Type: "libvirt", URI: "qemu:///system"}}
	if *cfg != want {
		t.Errorf("LoadConfig(includeDestination=false) = %+v, want %+v", *cfg, want)
	}
	cfg, err = LoadConfig(path, true)
	if err == nil || !strings.Contains(err.Error(), "cannot parse config file") {
		t.Fatalf("LoadConfig(includeDestination=true) error = %v, want decoding error", err)
	}
	if cfg != nil {
		t.Error("LoadConfig() returned a configuration on error")
	}
}

func TestLoadConfigValidatesDestination(t *testing.T) {
	tests := []struct {
		name      string
		change    func(*DestinationConfig)
		wantError string
	}{
		{name: "valid"},
		{name: "IPv4", change: func(c *DestinationConfig) { c.Server = "192.0.2.1" }},
		{name: "IPv6", change: func(c *DestinationConfig) { c.Server = "[2001:db8::1]" }},
		{name: "minimum port", change: func(c *DestinationConfig) { c.Port = 1 }},
		{name: "maximum port", change: func(c *DestinationConfig) { c.Port = 65535 }},
		{name: "root prefix", change: func(c *DestinationConfig) { c.Prefix = "/" }},
		{name: "missing organization", change: func(c *DestinationConfig) { c.OrgID = "" }, wantError: "destination.org_id is required"},
		{name: "blank organization", change: func(c *DestinationConfig) { c.OrgID = " \t\n" }, wantError: "destination.org_id is required"},
		{name: "missing server", change: func(c *DestinationConfig) { c.Server = "" }, wantError: "destination.server is required"},
		{name: "blank server", change: func(c *DestinationConfig) { c.Server = " \t\n" }, wantError: "destination.server is required"},
		{name: "server with scheme", change: func(c *DestinationConfig) { c.Server = "https://satellite.example.com" }, wantError: "destination.server must be"},
		{name: "server with credentials", change: func(c *DestinationConfig) { c.Server = "reporter:server-secret@satellite.example.com" }, wantError: "destination.server must be"},
		{name: "server with path", change: func(c *DestinationConfig) { c.Server = "satellite.example.com/candlepin" }, wantError: "destination.server must be"},
		{name: "server with query", change: func(c *DestinationConfig) { c.Server = "satellite.example.com?query" }, wantError: "destination.server must be"},
		{name: "server with fragment", change: func(c *DestinationConfig) { c.Server = "satellite.example.com#fragment" }, wantError: "destination.server must be"},
		{name: "server with port", change: func(c *DestinationConfig) { c.Server = "satellite.example.com:443" }, wantError: "destination.server must be"},
		{name: "server with whitespace", change: func(c *DestinationConfig) { c.Server = " satellite.example.com " }, wantError: "destination.server must be"},
		{name: "unbracketed IPv6", change: func(c *DestinationConfig) { c.Server = "2001:db8::1" }, wantError: "destination.server must be"},
		{name: "bracketed hostname", change: func(c *DestinationConfig) { c.Server = "[satellite.example.com]" }, wantError: "destination.server must be"},
		{name: "malformed server", change: func(c *DestinationConfig) { c.Server = "[2001:db8::1" }, wantError: "destination.server must be"},
		{name: "missing port", change: func(c *DestinationConfig) { c.Port = 0 }, wantError: "destination.port must be between 1 and 65535"},
		{name: "port too large", change: func(c *DestinationConfig) { c.Port = 65536 }, wantError: "destination.port must be between 1 and 65535"},
		{name: "missing prefix", change: func(c *DestinationConfig) { c.Prefix = "" }, wantError: "destination.prefix is required"},
		{name: "relative prefix", change: func(c *DestinationConfig) { c.Prefix = "subscription" }, wantError: "destination.prefix is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DestinationConfig{OrgID: "example-org", Server: "satellite.example.com", Port: 443, Prefix: "/subscription"}
			if tt.change != nil {
				tt.change(&cfg)
			}
			contents := fmt.Sprintf("[source]\ntype = 'libvirt'\nuri = 'qemu:///system'\n[destination]\norg_id = %q\nserver = %q\nport = %d\nprefix = %q\n", cfg.OrgID, cfg.Server, cfg.Port, cfg.Prefix)
			got, err := LoadConfig(writeConfig(t, contents), true)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("LoadConfig() error = %v", err)
				}
				if got.Destination != cfg {
					t.Errorf("LoadConfig() destination = %+v, want %+v", got.Destination, cfg)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("LoadConfig() error = %v, want %q", err, tt.wantError)
				}
				if strings.Contains(err.Error(), "server-secret") {
					t.Error("LoadConfig() exposed embedded credentials")
				}
				if got != nil {
					t.Error("LoadConfig() returned a configuration on error")
				}
			}
		})
	}
}

func writeConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "virtuoso.toml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
	return path
}
