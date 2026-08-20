package candlepin

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Peto-RH/virtuoso/internal/report"
)

func TestWriteHostGuestMapping(t *testing.T) {
	tests := []struct {
		name                string
		collectedHypervisor *report.Hypervisor
		wantJSON            string
	}{
		{
			name: "maps report to Candlepin format",
			collectedHypervisor: &report.Hypervisor{
				Source: "libvirt",
				ID:     "hypervisor-1",
				Name:   "hypervisor.example.com",
				Guests: []report.Guest{
					{ID: "blocked", State: report.GuestStateBlocked},
					{ID: "crashed", State: report.GuestStateCrashed},
					{ID: "paused", State: report.GuestStatePaused},
					{ID: "running", State: report.GuestStateRunning},
					{ID: "shutdown", State: report.GuestStateShutdown},
					{ID: "shutoff", State: report.GuestStateShutoff},
					{ID: "suspended", State: report.GuestStatePMSuspended},
					{ID: "unknown", State: report.GuestStateUnknown},
					{ID: "unsupported", State: report.GuestState(99)},
				},
			},
			wantJSON: `{
				"hypervisors": [{
					"hypervisorId": {"hypervisorId": "hypervisor-1"},
					"name": "hypervisor.example.com",
					"guestIds": [
						{"guestId": "blocked", "state": 2, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "crashed", "state": 6, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "paused", "state": 3, "attributes": {"virtWhoType": "libvirt", "active": 1}},
						{"guestId": "running", "state": 1, "attributes": {"virtWhoType": "libvirt", "active": 1}},
						{"guestId": "shutdown", "state": 4, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "shutoff", "state": 5, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "suspended", "state": 7, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "unknown", "state": 0, "attributes": {"virtWhoType": "libvirt", "active": 0}},
						{"guestId": "unsupported", "state": 0, "attributes": {"virtWhoType": "libvirt", "active": 0}}
					]
				}]
			}`,
		},
		{
			name: "omits empty name and writes empty guest list",
			collectedHypervisor: &report.Hypervisor{
				Source: "libvirt",
				ID:     "hypervisor-2",
			},
			wantJSON: `{
				"hypervisors": [{
					"hypervisorId": {"hypervisorId": "hypervisor-2"},
					"guestIds": []
				}]
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := WriteHostGuestMapping(&output, tt.collectedHypervisor); err != nil {
				t.Fatalf("WriteHostGuestMapping() error = %v", err)
			}

			assertJSONEqual(t, output.String(), tt.wantJSON)
		})
	}
}

func TestWriteHostGuestMapping_WriterError(t *testing.T) {
	writeErr := errors.New("write failed")
	err := WriteHostGuestMapping(errorWriter{err: writeErr}, &report.Hypervisor{})

	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteHostGuestMapping() error = %v, want wrapped %v", err, writeErr)
	}
	if !strings.Contains(err.Error(), "failed to encode host-guest mapping") {
		t.Errorf("WriteHostGuestMapping() error = %q, want encoding context", err)
	}
}

func assertJSONEqual(t *testing.T, gotJSON, wantJSON string) {
	t.Helper()

	var got any
	if err := json.Unmarshal([]byte(gotJSON), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, gotJSON)
	}

	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatalf("test expectation is not valid JSON: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("WriteHostGuestMapping() JSON = %s, want %s", gotJSON, wantJSON)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}
