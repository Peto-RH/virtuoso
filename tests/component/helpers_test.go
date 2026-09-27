//go:build component

package component_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const componentBinaryEnvironment = "VIRTUOSO_COMPONENT_BINARY"

// printedMapping mirrors the JSON shape of hostGuestMappingRequest in
// internal/destination/candlepin/models.go.
type printedMapping struct {
	Hypervisors []printedHypervisor `json:"hypervisors"`
}

// printedHypervisor mirrors the JSON shape of hypervisor and hypervisorID in
// internal/destination/candlepin/models.go.
type printedHypervisor struct {
	HypervisorID struct {
		HypervisorID string `json:"hypervisorId"`
	} `json:"hypervisorId"`
	Name   string         `json:"name"`
	Guests []printedGuest `json:"guestIds"`
}

// printedGuest mirrors the JSON shape of guest and guestAttributes in
// internal/destination/candlepin/models.go.
type printedGuest struct {
	GuestID    string `json:"guestId"`
	State      int    `json:"state"`
	Attributes struct {
		VirtWhoType string `json:"virtWhoType"`
		Active      int    `json:"active"`
	} `json:"attributes"`
}

func runComponentBinary(t *testing.T, configPath string, commandArgs ...string) ([]byte, []byte, error) {
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

	command := exec.CommandContext(ctx, binaryPath, append([]string{"--config", configPath}, commandArgs...)...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("virtuoso %s timed out: %v", strings.Join(commandArgs, " "), ctx.Err())
	}

	return stdout.Bytes(), stderr.Bytes(), err
}

func decodePrintedMapping(t *testing.T, stdout []byte) printedMapping {
	t.Helper()
	var mapping printedMapping
	decoder := json.NewDecoder(bytes.NewReader(stdout))
	if err := decoder.Decode(&mapping); err != nil {
		t.Fatalf("decode printed mapping: %v\nstdout:\n%s", err, stdout)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("printed mapping contains trailing JSON: %v\nstdout:\n%s", err, stdout)
	}
	return mapping
}
