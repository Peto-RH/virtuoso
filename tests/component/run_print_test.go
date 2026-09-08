//go:build component

package component_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const componentBinaryEnvironment = "VIRTUOSO_COMPONENT_BINARY"

type printedMapping struct {
	Hypervisors []printedHypervisor `json:"hypervisors"`
}

type printedHypervisor struct {
	HypervisorID struct {
		HypervisorID string `json:"hypervisorId"`
	} `json:"hypervisorId"`
	Name   string         `json:"name"`
	Guests []printedGuest `json:"guestIds"`
}

type printedGuest struct {
	GuestID    string `json:"guestId"`
	State      int    `json:"state"`
	Attributes struct {
		VirtWhoType string `json:"virtWhoType"`
		Active      int    `json:"active"`
	} `json:"attributes"`
}

func TestComponentRunPrint(t *testing.T) {
	configPath := writeComponentConfig(t, libvirtTestURI(t))
	stdout, stderr, err := runComponentBinary(t, configPath)
	if err != nil {
		t.Fatalf("virtuoso run --print error = %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	var mapping printedMapping
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	if err := decoder.Decode(&mapping); err != nil {
		t.Fatalf("decode printed mapping: %v\nstdout:\n%s", err, stdout)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("printed mapping contains trailing JSON: %v\nstdout:\n%s", err, stdout)
	}

	if len(mapping.Hypervisors) != 1 {
		t.Fatalf("printed hypervisor count = %d, want 1", len(mapping.Hypervisors))
	}
	hypervisor := mapping.Hypervisors[0]
	if hypervisor.HypervisorID.HypervisorID == "" {
		t.Error("printed hypervisor ID is empty")
	}
	if hypervisor.Name != hypervisor.HypervisorID.HypervisorID {
		t.Errorf("printed hypervisor name = %q, want ID %q", hypervisor.Name, hypervisor.HypervisorID.HypervisorID)
	}

	wantGuests := map[string]struct {
		state  int
		active int
	}{
		"11111111-1111-1111-1111-111111111111": {state: 1, active: 1},
		"22222222-2222-2222-2222-222222222222": {state: 3, active: 1},
		"33333333-3333-3333-3333-333333333333": {state: 5, active: 0},
	}
	if len(hypervisor.Guests) != len(wantGuests) {
		t.Fatalf("printed guest count = %d, want %d", len(hypervisor.Guests), len(wantGuests))
	}
	for _, guest := range hypervisor.Guests {
		want, ok := wantGuests[guest.GuestID]
		if !ok {
			t.Errorf("printed mapping contains unexpected guest %q", guest.GuestID)
			continue
		}
		if guest.State != want.state {
			t.Errorf("printed guest %q state = %d, want %d", guest.GuestID, guest.State, want.state)
		}
		if guest.Attributes.Active != want.active {
			t.Errorf("printed guest %q active = %d, want %d", guest.GuestID, guest.Attributes.Active, want.active)
		}
		if guest.Attributes.VirtWhoType != "libvirt" {
			t.Errorf("printed guest %q virtWhoType = %q, want %q", guest.GuestID, guest.Attributes.VirtWhoType, "libvirt")
		}
		delete(wantGuests, guest.GuestID)
	}
	for guestID := range wantGuests {
		t.Errorf("printed mapping does not contain guest %q", guestID)
	}
}

func TestComponentRunPrintSourceFailure(t *testing.T) {
	missingFixture := filepath.Join(t.TempDir(), "missing-node.xml")
	configPath := writeComponentConfig(t, "test://"+filepath.ToSlash(missingFixture))
	stdout, stderr, err := runComponentBinary(t, configPath)
	if err == nil {
		t.Fatalf("virtuoso run --print error = nil\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}

	output := string(stdout) + string(stderr)
	if !strings.Contains(output, "cannot connect to libvirt") {
		t.Errorf("virtuoso output does not describe the source failure\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func runComponentBinary(t *testing.T, configPath string) ([]byte, []byte, error) {
	t.Helper()

	binaryPath := os.Getenv(componentBinaryEnvironment)
	if binaryPath == "" {
		t.Fatalf("%s is required", componentBinaryEnvironment)
	}
	if !filepath.IsAbs(binaryPath) {
		t.Fatalf("%s must be an absolute path, got %q", componentBinaryEnvironment, binaryPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, binaryPath, "--config", configPath, "run", "--print")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("virtuoso run --print timed out: %v", ctx.Err())
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

func writeComponentConfig(t *testing.T, sourceURI string) string {
	t.Helper()

	configPath := filepath.Join(t.TempDir(), "virtuoso.toml")
	contents := fmt.Sprintf("[source]\ntype = \"libvirt\"\nuri = %q\n", sourceURI)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write component config: %v", err)
	}

	return configPath
}

func libvirtTestURI(t *testing.T) string {
	t.Helper()

	fixturePath, err := filepath.Abs(filepath.Join("testdata", "libvirt", "node.xml"))
	if err != nil {
		t.Fatalf("resolve libvirt fixture path: %v", err)
	}

	return "test://" + filepath.ToSlash(fixturePath)
}
