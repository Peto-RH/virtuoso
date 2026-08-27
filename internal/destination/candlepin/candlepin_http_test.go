package candlepin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Peto-RH/virtuoso/internal/report"
)

const (
	testOrgID         = "test-org"
	testCorrelationID = "test-correlation-id"
	testReporterID    = "test-reporter-id"
)

func TestCandlepinClientPing(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantError  string
	}{
		{name: "accessible organization", statusCode: http.StatusOK},
		{
			name:       "inaccessible organization",
			statusCode: http.StatusNotFound,
			wantError:  "org_id 'test-org' not found or not accessible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var requestCount atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				requestCount.Add(1)
				assertRequest(
					t,
					request,
					http.MethodPut,
					"/candlepin/hypervisors/test-org/heartbeat",
					url.Values{"reporter_id": {"status_test"}},
					"",
				)
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			client := newTestCandlepinClient(server.Client(), server.URL+"/candlepin")
			defer func() {
				_ = client.Close()
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			err := client.Ping(ctx)

			if requestCount.Load() != 1 {
				t.Errorf("Ping() request count = %d, want 1", requestCount.Load())
			}
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("Ping() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Ping() error = nil, want %q", tt.wantError)
			}
			if err.Error() != tt.wantError {
				t.Errorf("Ping() error = %q, want %q", err, tt.wantError)
			}
		})
	}
}

func TestCandlepinClientSend(t *testing.T) {
	const jobID = "test-job-id"

	var reportRequestCount atomic.Int32
	var jobRequestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/candlepin/hypervisors/test-org":
			reportRequestCount.Add(1)
			assertRequest(
				t,
				request,
				http.MethodPost,
				"/candlepin/hypervisors/test-org",
				url.Values{"reporter_id": {testReporterID}},
				"text/plain",
			)
			assertRequestJSON(t, request, `{
				"hypervisors": [{
					"hypervisorId": {"hypervisorId": "hypervisor-1"},
					"name": "hypervisor.example.test",
					"guestIds": [
						{
							"guestId": "running-guest",
							"state": 1,
							"attributes": {"virtWhoType": "libvirt", "active": 1}
						},
						{
							"guestId": "stopped-guest",
							"state": 5,
							"attributes": {"virtWhoType": "libvirt", "active": 0}
						}
					]
				}]
			}`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"id":%q}`, jobID)
		case request.Method == http.MethodGet && request.URL.Path == "/candlepin/jobs/"+jobID:
			jobRequestCount.Add(1)
			assertRequest(
				t,
				request,
				http.MethodGet,
				"/candlepin/jobs/"+jobID,
				nil,
				"",
			)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"state":"FINISHED"}`)
		default:
			t.Errorf("unexpected Candlepin request: %s %s", request.Method, request.URL.String())
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestCandlepinClient(server.Client(), server.URL+"/candlepin")
	defer func() {
		_ = client.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err := client.Send(ctx, &report.Hypervisor{
		Source: "libvirt",
		ID:     "hypervisor-1",
		Name:   "hypervisor.example.test",
		Guests: []report.Guest{
			{ID: "running-guest", State: report.GuestStateRunning},
			{ID: "stopped-guest", State: report.GuestStateShutoff},
		},
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	if reportRequestCount.Load() != 1 {
		t.Errorf("Send() report request count = %d, want 1", reportRequestCount.Load())
	}
	if jobRequestCount.Load() != 1 {
		t.Errorf("Send() job request count = %d, want 1", jobRequestCount.Load())
	}
}

func newTestCandlepinClient(httpClient *http.Client, baseURL string) *CandlepinClient {
	return &CandlepinClient{
		httpClient:    httpClient,
		baseURL:       baseURL,
		orgID:         testOrgID,
		correlationID: testCorrelationID,
		reporterID:    testReporterID,
	}
}

func assertRequest(
	t *testing.T,
	request *http.Request,
	wantMethod string,
	wantPath string,
	wantQuery url.Values,
	wantContentType string,
) {
	t.Helper()

	if request.Method != wantMethod {
		t.Errorf("request method = %q, want %q", request.Method, wantMethod)
	}
	if request.URL.Path != wantPath {
		t.Errorf("request path = %q, want %q", request.URL.Path, wantPath)
	}
	if request.URL.Query().Encode() != wantQuery.Encode() {
		t.Errorf("request query = %v, want %v", request.URL.Query(), wantQuery)
	}

	wantHeaders := map[string]string{
		"Accept":           "application/json",
		"Content-Type":     wantContentType,
		"User-Agent":       "virtuoso/0.1.0",
		"X-Correlation-Id": testCorrelationID,
	}
	for name, want := range wantHeaders {
		if got := request.Header.Get(name); got != want {
			t.Errorf("request header %s = %q, want %q", name, got, want)
		}
	}
}

func assertRequestJSON(t *testing.T, request *http.Request, wantJSON string) {
	t.Helper()

	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read request body: %v", err)
		return
	}

	var got any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Errorf("request body is not valid JSON: %v\n%s", err, body)
		return
	}

	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Errorf("test expectation is not valid JSON: %v", err)
		return
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("request JSON = %s, want %s", body, wantJSON)
	}
}
