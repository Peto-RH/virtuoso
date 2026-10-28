package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/report"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

func TestReadVMwarePassword(t *testing.T) {
	tests := []struct {
		name       string
		credential string
		want       string
		wantError  bool
	}{
		{name: "unchanged", credential: "password", want: "password"},
		{name: "one final LF", credential: "password\n", want: "password"},
		{name: "one final CRLF", credential: "password\r\n", want: "password"},
		{name: "bare final CR", credential: "password\r", want: "password\r"},
		{name: "surrounding spaces", credential: " password \n", want: " password "},
		{name: "whitespace only", credential: " \t \r\n", want: " \t "},
		{name: "repeated LF", credential: "password\n\n", want: "password\n"},
		{name: "repeated CRLF", credential: "password\r\n\r\n", want: "password\r\n"},
		{name: "embedded newline", credential: "pass\nword\n", want: "pass\nword"},
		{name: "empty", wantError: true},
		{name: "LF only", credential: "\n", wantError: true},
		{name: "CRLF only", credential: "\r\n", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credentialsDir := t.TempDir()
			t.Setenv("CREDENTIALS_DIRECTORY", credentialsDir)
			if err := os.WriteFile(filepath.Join(credentialsDir, "virtuoso.vmware.password"), []byte(tt.credential), 0o600); err != nil {
				t.Fatal(err)
			}

			password, err := readVMwarePassword()
			if tt.wantError {
				if err == nil {
					t.Fatal("readVMwarePassword() error = nil, want empty credential error")
				}
				if !strings.Contains(err.Error(), "empty") || !strings.Contains(err.Error(), vmwarePasswordCredentialName) {
					t.Errorf("readVMwarePassword() error = %q, want empty credential context", err)
				}
				if password != "" {
					t.Error("readVMwarePassword() returned a password on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("readVMwarePassword() error = %v", err)
			}
			if password != tt.want {
				t.Errorf("readVMwarePassword() = %q, want %q", password, tt.want)
			}
		})
	}
}

func TestReadVMwarePasswordErrors(t *testing.T) {
	t.Run("missing credentials directory", func(t *testing.T) {
		t.Setenv("CREDENTIALS_DIRECTORY", "")
		password, err := readVMwarePassword()
		if err == nil || !strings.Contains(err.Error(), "CREDENTIALS_DIRECTORY") {
			t.Fatalf("readVMwarePassword() error = %v, want credentials directory context", err)
		}
		if password != "" {
			t.Error("readVMwarePassword() returned a password on error")
		}
	})

	t.Run("missing runtime file", func(t *testing.T) {
		t.Setenv("CREDENTIALS_DIRECTORY", t.TempDir())
		password, err := readVMwarePassword()
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("readVMwarePassword() error = %v, want wrapped %v", err, os.ErrNotExist)
		}
		if !strings.Contains(err.Error(), vmwarePasswordCredentialName) {
			t.Errorf("readVMwarePassword() error = %q, want runtime credential context", err)
		}
		if password != "" {
			t.Error("readVMwarePassword() returned a password on error")
		}
	})
}

func TestNewVMwareProviderRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("CREDENTIALS_DIRECTORY", "")
	tests := []struct {
		name, uri, username, wantError string
	}{
		{name: "malformed URI", uri: "https://user:uri-secret@host/%zz", username: "reporter", wantError: "valid absolute HTTPS URI"},
		{name: "relative URI", uri: "/sdk", username: "reporter", wantError: "absolute HTTPS URI"},
		{name: "plaintext HTTP", uri: "http://vcenter.example/sdk", username: "reporter", wantError: "absolute HTTPS URI"},
		{name: "missing hostname", uri: "https:///sdk", username: "reporter", wantError: "hostname"},
		{name: "embedded password", uri: "https://user:uri-secret@vcenter.example/sdk", username: "reporter", wantError: "must not contain credentials"},
		{name: "embedded username", uri: "https://user@vcenter.example/sdk", username: "reporter", wantError: "must not contain credentials"},
		{name: "missing username", uri: "https://vcenter.example/sdk", wantError: "source.username is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewVMwareProvider(context.Background(), &config.SourceConfig{URI: tt.uri, Username: tt.username})
			if err == nil || !strings.Contains(err.Error(), tt.wantError) {
				t.Fatalf("NewVMwareProvider() error = %v, want %q before reading credentials or connecting", err, tt.wantError)
			}
			if p != nil {
				t.Error("NewVMwareProvider() returned a provider on error")
			}
			if strings.Contains(err.Error(), "uri-secret") {
				t.Error("configuration error exposed embedded password")
			}
		})
	}
}

func TestSelectVMwareHostsAndVMRefs(t *testing.T) {
	tests := []struct {
		name     string
		change   func(*mo.HostSystem)
		eligible bool
	}{
		{name: "eligible", eligible: true},
		{name: "missing hardware", change: func(h *mo.HostSystem) { h.Hardware = nil }},
		{name: "empty hardware ID", change: func(h *mo.HostSystem) { h.Hardware.SystemInfo.Uuid = "" }},
		{name: "blank hardware ID", change: func(h *mo.HostSystem) { h.Hardware.SystemInfo.Uuid = " \t\n" }},
		{name: "missing config", change: func(h *mo.HostSystem) { h.Config = nil }},
		{name: "missing network", change: func(h *mo.HostSystem) { h.Config.Network = nil }},
		{name: "missing DNS config", change: func(h *mo.HostSystem) { h.Config.Network.DnsConfig = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := vmwareTestHost("candidate")
			host.Vm = []types.ManagedObjectReference{{Type: "VirtualMachine", Value: "candidate-vm"}}
			if tt.change != nil {
				tt.change(&host)
			}
			emptyHost := vmwareTestHost("empty-host")
			wantHosts := []mo.HostSystem{emptyHost}
			var wantRefs []types.ManagedObjectReference
			if tt.eligible {
				wantHosts = append([]mo.HostSystem{host}, wantHosts...)
				wantRefs = host.Vm
			}

			hosts, refs := selectVMwareHostsAndVMRefs([]mo.HostSystem{host, emptyHost})
			if !reflect.DeepEqual(hosts, wantHosts) {
				t.Errorf("selected hosts = %+v, want %+v", hosts, wantHosts)
			}
			if !reflect.DeepEqual(refs, wantRefs) {
				t.Errorf("guest references = %v, want %v", refs, wantRefs)
			}
		})
	}
}

func TestHypervisorsFromVMwareHostsNamesAndIDs(t *testing.T) {
	tests := []struct{ name, hostname, domain, wantName string }{
		{name: "hostname and domain", hostname: "esxi", domain: "example.com", wantName: "esxi.example.com"},
		{name: "no domain", hostname: "esxi", wantName: "esxi"},
		{name: "literal whitespace", hostname: " esxi ", domain: " example.com ", wantName: " esxi . example.com "},
		{name: "empty hostname", domain: "example.com", wantName: ".example.com"},
		{name: "empty DNS values"},
		{name: "blank hostname", hostname: " \t ", wantName: " \t "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host := vmwareTestHost("host-1")
			host.Config.Network.DnsConfig = &types.HostDnsConfig{HostName: tt.hostname, DomainName: tt.domain}
			got := hypervisorsFromVMwareHosts([]mo.HostSystem{host}, nil)
			want := []report.Hypervisor{{Source: "esx", ID: " literal hardware ID ", Name: tt.wantName, Guests: []report.Guest{}}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("hypervisors = %+v, want %+v", got, want)
			}
		})
	}
}

func TestGuestsForVMwareHostIdentifiers(t *testing.T) {
	const biosUUID = "01234567-89ab-cdef-1234-567890abcdef"
	tests := []struct {
		name      string
		vm        mo.VirtualMachine
		missing   bool
		wantGuest bool
	}{
		{name: "BIOS UUID preserves byte order", vm: mo.VirtualMachine{Config: &types.VirtualMachineConfigInfo{Uuid: " 01234567-89AB-CDEF-1234-567890ABCDEF ", InstanceUuid: "00000000-0000-0000-0000-000000000001"}}, wantGuest: true},
		{name: "absent reference", missing: true},
		{name: "missing configuration"},
		{name: "missing BIOS UUID", vm: mo.VirtualMachine{Config: &types.VirtualMachineConfigInfo{InstanceUuid: biosUUID}}},
		{name: "invalid BIOS UUID", vm: mo.VirtualMachine{Config: &types.VirtualMachineConfigInfo{Uuid: "invalid", InstanceUuid: biosUUID}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidateRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: "candidate"}
			validRef := types.ManagedObjectReference{Type: "VirtualMachine", Value: "valid-neighbor"}
			host := vmwareTestHost("host")
			host.Vm = []types.ManagedObjectReference{candidateRef, validRef}
			neighbor := report.Guest{ID: "42231ec1-e482-06b2-a798-a51ff68f0d4b", State: report.GuestStateShutoff}
			vms := map[types.ManagedObjectReference]mo.VirtualMachine{
				validRef: {Config: &types.VirtualMachineConfigInfo{Uuid: neighbor.ID}, Runtime: types.VirtualMachineRuntimeInfo{PowerState: types.VirtualMachinePowerStatePoweredOff}},
			}
			if !tt.missing {
				vms[candidateRef] = tt.vm
			}
			want := []report.Guest{}
			if tt.wantGuest {
				want = append(want, report.Guest{ID: biosUUID, State: report.GuestStateUnknown})
			}
			want = append(want, neighbor)
			if got := guestsForVMwareHost(host, vms); !reflect.DeepEqual(got, want) {
				t.Errorf("guests = %+v, want %+v", got, want)
			}
		})
	}
}

func TestGuestStateFromVMware(t *testing.T) {
	tests := []struct {
		name  string
		state types.VirtualMachinePowerState
		want  report.GuestState
	}{
		{name: "powered on", state: types.VirtualMachinePowerStatePoweredOn, want: report.GuestStateRunning},
		{name: "suspended", state: types.VirtualMachinePowerStateSuspended, want: report.GuestStatePaused},
		{name: "powered off", state: types.VirtualMachinePowerStatePoweredOff, want: report.GuestStateShutoff},
		{name: "unrecognized", state: types.VirtualMachinePowerState("unrecognized"), want: report.GuestStateUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := guestStateFromVMware(tt.state); got != tt.want {
				t.Errorf("guestStateFromVMware(%q) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestLogVMwareSessionFailure(t *testing.T) {
	const secret = "server-echoed-password"
	fault := &soap.Fault{Code: secret, String: secret}
	fault.Detail.Fault = &types.RuntimeFault{MethodFault: types.MethodFault{
		FaultMessage: []types.LocalizableMessage{{Message: secret}},
	}}
	tests := []struct {
		name      string
		operation string
		err       error
		wantType  string
	}{
		{name: "transport error", operation: "login", err: vmwareSessionTestError(secret), wantType: "provider.vmwareSessionTestError"},
		{name: "typed SOAP fault", operation: "login", err: soap.WrapSoapFault(fault), wantType: "*types.RuntimeFault"},
		{name: "SOAP fault without detail", operation: "logout", err: soap.WrapSoapFault(&soap.Fault{Code: secret, String: secret})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			originalLogger := slog.Default()
			t.Cleanup(func() { slog.SetDefault(originalLogger) })
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})))

			logVMwareSessionFailure(tt.operation, tt.err)

			var event struct {
				Level     string `json:"level"`
				Operation string `json:"operation"`
				ErrorType string `json:"error_type"`
			}
			if err := json.Unmarshal(output.Bytes(), &event); err != nil {
				t.Fatalf("invalid log JSON: %v", err)
			}
			if event.Level != "DEBUG" {
				t.Errorf("unexpected log event: %+v", event)
			}
			if event.Operation != tt.operation || event.ErrorType == "" {
				t.Errorf("log context = operation %q, error_type %q; want operation %q and a diagnostic type", event.Operation, event.ErrorType, tt.operation)
			}
			if tt.wantType != "" && event.ErrorType != tt.wantType {
				t.Errorf("error_type = %q, want %q", event.ErrorType, tt.wantType)
			}
			if strings.Contains(output.String(), secret) {
				t.Error("session failure log contains server-controlled error content")
			}
		})
	}
}

type vmwareSessionTestError string

func (e vmwareSessionTestError) Error() string { return string(e) }

func vmwareTestHost(ref string) mo.HostSystem {
	return mo.HostSystem{
		ManagedEntity: mo.ManagedEntity{ExtensibleManagedObject: mo.ExtensibleManagedObject{Self: types.ManagedObjectReference{Type: "HostSystem", Value: ref}}, Name: "inventory-name-must-not-be-used"},
		Hardware:      &types.HostHardwareInfo{SystemInfo: types.HostSystemInfo{Uuid: " literal hardware ID "}},
		Config:        &types.HostConfigInfo{Network: &types.HostNetworkInfo{DnsConfig: &types.HostDnsConfig{HostName: "esxi", DomainName: "example.com"}}},
	}
}
