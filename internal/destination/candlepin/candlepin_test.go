package candlepin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Peto-RH/virtuoso/internal/report"
)

func TestWriteHostGuestMapping(t *testing.T) {
	tests := []struct {
		name                 string
		collectedHypervisors []report.Hypervisor
		wantJSON             string
	}{
		{
			name: "maps report to Candlepin format",
			collectedHypervisors: []report.Hypervisor{{
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
			}},
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
			collectedHypervisors: []report.Hypervisor{{
				Source: "libvirt",
				ID:     "hypervisor-2",
			}},
			wantJSON: `{
				"hypervisors": [{
					"hypervisorId": {"hypervisorId": "hypervisor-2"},
					"guestIds": []
				}]
			}`,
		},
		{
			name:     "nil collection writes an empty array",
			wantJSON: `{"hypervisors": []}`,
		},
		{
			name:                 "empty collection writes an empty array",
			collectedHypervisors: []report.Hypervisor{},
			wantJSON:             `{"hypervisors": []}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			if err := WriteHostGuestMapping(&output, tt.collectedHypervisors); err != nil {
				t.Fatalf("WriteHostGuestMapping() error = %v", err)
			}

			assertJSONEqual(t, output.String(), tt.wantJSON)
		})
	}
}

func TestWriteHostGuestMappingMultipleHypervisors(t *testing.T) {
	collectedHypervisors := []report.Hypervisor{
		{
			Source: "libvirt",
			ID:     "host-1-uuid",
			Name:   "esxi-1.example.com",
			Guests: []report.Guest{
				{ID: "running-vm", State: report.GuestStateRunning},
			},
		},
		{
			Source: "esx",
			ID:     "host-2-uuid",
			Name:   "esxi-2.example.com",
			Guests: []report.Guest{
				{ID: "paused-vm", State: report.GuestStatePaused},
			},
		},
	}

	var output bytes.Buffer
	if err := WriteHostGuestMapping(&output, collectedHypervisors); err != nil {
		t.Fatalf("WriteHostGuestMapping() error = %v", err)
	}

	assertJSONEqual(t, output.String(), `{
		"hypervisors": [
			{
				"hypervisorId": {"hypervisorId": "host-1-uuid"},
				"name": "esxi-1.example.com",
				"guestIds": [
					{"guestId": "running-vm", "state": 1, "attributes": {"virtWhoType": "libvirt", "active": 1}}
				]
			},
			{
				"hypervisorId": {"hypervisorId": "host-2-uuid"},
				"name": "esxi-2.example.com",
				"guestIds": [
					{"guestId": "paused-vm", "state": 3, "attributes": {"virtWhoType": "esx", "active": 1}}
				]
			}
		]
	}`)
}

func TestSendMultipleHypervisorsInOneRequest(t *testing.T) {
	postedMappings := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/hypervisors/org":
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			select {
			case postedMappings <- string(body):
			default:
				http.Error(w, "too many report requests", http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"id":"job-1"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/jobs/job-1":
			_, _ = io.WriteString(w, `{"state":"FINISHED"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &CandlepinClient{
		httpClient: server.Client(),
		baseURL:    server.URL,
		orgID:      "org",
		reporterID: "reporter",
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	hypervisors := []report.Hypervisor{
		{Source: "esx", ID: "host-1", Guests: []report.Guest{{ID: "vm-1", State: report.GuestStateRunning}}},
		{Source: "esx", ID: "host-2"},
	}
	if err := client.Send(ctx, hypervisors); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(postedMappings) != 1 {
		t.Fatalf("Send() made %d report requests, want 1", len(postedMappings))
	}
	assertJSONEqual(t, <-postedMappings, `{
		"hypervisors": [
			{
				"hypervisorId": {"hypervisorId": "host-1"},
				"guestIds": [
					{"guestId": "vm-1", "state": 1, "attributes": {"virtWhoType": "esx", "active": 1}}
				]
			},
			{
				"hypervisorId": {"hypervisorId": "host-2"},
				"guestIds": []
			}
		]
	}`)
}

func TestWriteHostGuestMapping_WriterError(t *testing.T) {
	writeErr := errors.New("write failed")
	err := WriteHostGuestMapping(errorWriter{err: writeErr}, []report.Hypervisor{{}})

	if !errors.Is(err, writeErr) {
		t.Fatalf("WriteHostGuestMapping() error = %v, want wrapped %v", err, writeErr)
	}
	if !strings.Contains(err.Error(), "failed to encode host-guest mapping") {
		t.Errorf("WriteHostGuestMapping() error = %q, want encoding context", err)
	}
}

func TestHandleHTTPError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
		retryAfter string
		wantError  string
	}{
		{name: "OK", statusCode: http.StatusOK},
		{name: "successful upper boundary", statusCode: 299},
		{
			name:       "consumer certificate gone",
			statusCode: http.StatusGone,
			wantError:  "consumer certificate invalid (HTTP 410 Gone) - system may need re-registration",
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			retryAfter: "120",
			wantError:  "rate limited (HTTP 429), retry after: 120",
		},
		{
			name:       "generic HTTP error",
			statusCode: http.StatusServiceUnavailable,
			body:       "service unavailable",
			wantError:  "HTTP 503: service unavailable",
		},
	}

	client := &CandlepinClient{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := &http.Response{
				StatusCode: tt.statusCode,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(tt.body)),
			}
			response.Header.Set("Retry-After", tt.retryAfter)

			err := client.handleHTTPError(response)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("handleHTTPError() error = %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("handleHTTPError() error = nil, want %q", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Errorf("handleHTTPError() error = %q, want %q", err, tt.wantError)
			}
		})
	}
}

func TestHandleHTTPError_TruncatesResponseBody(t *testing.T) {
	body := strings.Repeat("x", 1025)
	response := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	err := (&CandlepinClient{}).handleHTTPError(response)
	wantError := "HTTP 500: " + strings.Repeat("x", 1024)
	if err == nil {
		t.Fatalf("handleHTTPError() error = nil, want %q", wantError)
	}
	if err.Error() != wantError {
		t.Errorf("handleHTTPError() error = %q, want %q", err, wantError)
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
		t.Errorf("mapping JSON = %s, want %s", gotJSON, wantJSON)
	}
}

type errorWriter struct {
	err error
}

func (w errorWriter) Write([]byte) (int, error) {
	return 0, w.err
}

func TestEvaluateJobState(t *testing.T) {
	tests := []struct {
		name         string
		state        string
		wantFinished bool
		wantError    string
	}{
		{name: "finished", state: "FINISHED", wantFinished: true},
		{name: "failed", state: "FAILED", wantError: "job job-123: FAILED"},
		{name: "canceled", state: "CANCELED", wantError: "job job-123: CANCELED"},
		{name: "created", state: "CREATED"},
		{name: "waiting", state: "WAITING"},
		{name: "running", state: "RUNNING"},
		{name: "unknown", state: "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finished, err := evaluateJobState("job-123", tt.state)
			if finished != tt.wantFinished {
				t.Errorf("evaluateJobState() finished = %t, want %t", finished, tt.wantFinished)
			}

			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("evaluateJobState() error = %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("evaluateJobState() error = nil, want %q", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Errorf("evaluateJobState() error = %q, want %q", err, tt.wantError)
			}
		})
	}
}
