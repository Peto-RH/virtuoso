//go:build component

package component_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComponentRejectsInvalidConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "virtuoso.toml")
	contents := "[source]\ntype = 'unsupported'\nuri = 'test:///default'\n"
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("write component config: %v", err)
	}
	for _, args := range [][]string{{"run", "--print"}, {"status"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr, err := runComponentBinary(t, configPath, args...)
			if err == nil {
				t.Fatalf("virtuoso accepted invalid configuration\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
			if !strings.Contains(string(stdout)+string(stderr), `unsupported source.type "unsupported"`) {
				t.Errorf("virtuoso did not report the invalid source type\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
		})
	}
}

func TestComponentRejectsInvalidDestination(t *testing.T) {
	configPath := writeLibvirtComponentConfig(t, libvirtTestURI(t))
	for _, command := range []string{"run", "status"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, err := runComponentBinary(t, configPath, command)
			if err == nil {
				t.Fatalf("virtuoso %s accepted a missing destination\nstdout:\n%s\nstderr:\n%s", command, stdout, stderr)
			}
			if !strings.Contains(string(stdout)+string(stderr), "destination.org_id is required") {
				t.Errorf("virtuoso did not report the missing destination\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
		})
	}
}

func TestComponentRunPrintIgnoresDestinationFields(t *testing.T) {
	configPath := writeLibvirtComponentConfig(t, libvirtTestURI(t))
	contents, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read component config: %v", err)
	}
	contents = append(contents, "\n[destination]\nport = 'invalid'\n"...)
	if err := os.WriteFile(configPath, contents, 0o600); err != nil {
		t.Fatalf("write component config: %v", err)
	}

	t.Run("run --print", func(t *testing.T) {
		stdout, stderr, err := runComponentBinary(t, configPath, "run", "--print")
		if err != nil {
			t.Fatalf("print mode required valid destination fields: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
		}
		mapping := decodePrintedMapping(t, stdout)
		if len(mapping.Hypervisors) != 1 || len(mapping.Hypervisors[0].Guests) != 3 {
			t.Errorf("libvirt did not collect the expected host and guests: %+v", mapping)
		}
	})
	for _, command := range []string{"run", "status"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr, err := runComponentBinary(t, configPath, command)
			if err == nil {
				t.Fatalf("virtuoso %s accepted an invalid destination field type\nstdout:\n%s\nstderr:\n%s", command, stdout, stderr)
			}
			if !strings.Contains(string(stdout)+string(stderr), "cannot parse config file") {
				t.Errorf("virtuoso did not report the destination field type error\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
			}
		})
	}
}

func TestComponentStatusChecksSourceBeforeDestination(t *testing.T) {
	missingFixture := filepath.Join(t.TempDir(), "missing-node.xml")
	configPath := writeLibvirtComponentConfig(t, "test://"+filepath.ToSlash(missingFixture))
	stdout, stderr, err := runComponentBinary(t, configPath, "status")
	if err == nil {
		t.Fatalf("status succeeded with an unavailable source\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
	if !strings.Contains(string(stdout)+string(stderr), "source validation failed: cannot connect to libvirt") {
		t.Errorf("status did not report the source failure before the missing destination\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}
