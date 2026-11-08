package candlepin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Peto-RH/virtuoso/internal/config"
	"github.com/Peto-RH/virtuoso/internal/report"
	"github.com/google/uuid"
)

const (
	userAgent       = "virtuoso/0.1.0"
	jobPollInterval = 5 * time.Second
	jobPollTimeout  = 5 * time.Minute
	certPath        = "/etc/pki/consumer/cert.pem"
	keyPath         = "/etc/pki/consumer/key.pem"
	machineIDPath   = "/etc/machine-id"
)

type CandlepinClient struct {
	httpClient    *http.Client
	baseURL       string
	orgID         string
	correlationID string
	reporterID    string // hostname + "-" + machine-id
}

func generateReporterID() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "unknown"
	}

	machineIDBytes, err := os.ReadFile(machineIDPath)
	if err != nil {
		return hostname
	}

	machineID := strings.TrimSpace(string(machineIDBytes))
	if machineID == "" {
		return hostname
	}

	return hostname + "-" + machineID
}

func NewCandlepinClient(destConfig *config.DestinationConfig) (*CandlepinClient, error) {
	slog.Debug("initializing Candlepin client")

	client, err := newHTTPClient(certPath, keyPath)
	if err != nil {
		return nil, err
	}

	baseURL := fmt.Sprintf("https://%s:%d%s", destConfig.Server, destConfig.Port, strings.TrimRight(destConfig.Prefix, "/"))
	slog.Debug("Candlepin client base URL", "base_url", baseURL)
	correlationID := uuid.New().String()
	reporterID := generateReporterID()

	slog.Debug("Candlepin client initialized",
		"server", destConfig.Server,
		"port", destConfig.Port,
		"org_id", destConfig.OrgID,
		"correlation_id", correlationID,
		"reporter_id", reporterID)

	return &CandlepinClient{
		httpClient:    client,
		baseURL:       baseURL,
		orgID:         destConfig.OrgID,
		correlationID: correlationID,
		reporterID:    reporterID,
	}, nil
}

func (c *CandlepinClient) Send(ctx context.Context, hypervisors []report.Hypervisor) error {
	if len(hypervisors) == 0 {
		slog.Info("no hypervisors collected, sending heartbeat")
		return c.Heartbeat(ctx)
	}

	slog.Debug("sending hypervisor data", "hosts", len(hypervisors))

	requestBody := newHostGuestMappingRequest(hypervisors)

	jobID, err := c.reportHostGuestMapping(ctx, requestBody)
	if err != nil {
		return fmt.Errorf("failed to report host-guest mapping: %w", err)
	}

	slog.Debug("host-guest mapping submitted", "job_id", jobID)

	if err := c.waitForJobCompletion(ctx, jobID); err != nil {
		return fmt.Errorf("job polling failed: %w", err)
	}

	slog.Debug("hypervisor data sent successfully")
	return nil
}

func (c *CandlepinClient) Heartbeat(ctx context.Context) error {
	slog.Debug("sending Candlepin heartbeat", "reporter_id", c.reporterID)
	url := fmt.Sprintf("%s/hypervisors/%s/heartbeat?reporter_id=%s",
		c.baseURL, url.PathEscape(c.orgID), url.QueryEscape(c.reporterID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, nil)
	if err != nil {
		return fmt.Errorf("cannot create heartbeat request: %w", err)
	}
	jobID, err := c.submitJob(req)
	if err != nil {
		return fmt.Errorf("failed to send heartbeat: %w", err)
	}

	slog.Debug("heartbeat submitted", "job_id", jobID)
	// Acceptance only queues the heartbeat; timestamps are updated by the job.
	if err := c.waitForJobCompletion(ctx, jobID); err != nil {
		return fmt.Errorf("heartbeat job polling failed: %w", err)
	}
	return nil
}

func WriteHostGuestMapping(w io.Writer, hypervisors []report.Hypervisor) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(newHostGuestMappingRequest(hypervisors)); err != nil {
		return fmt.Errorf("failed to encode host-guest mapping: %w", err)
	}
	return nil
}

func (c *CandlepinClient) Ping(ctx context.Context) error {
	slog.Debug("testing Candlepin connection and org access")

	url := fmt.Sprintf("%s/hypervisors/%s/heartbeat?reporter_id=status_test",
		c.baseURL, url.PathEscape(c.orgID))
	req, err := http.NewRequestWithContext(ctx, "PUT", url, nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("connection test failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("org_id '%s' not found or not accessible", c.orgID)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Debug("error response body", "status", resp.StatusCode, "body", string(body))
		body = body[:min(len(body), 1024)]
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *CandlepinClient) Close() error {
	slog.Debug("closing Candlepin client")
	c.httpClient.CloseIdleConnections()
	return nil
}

func (c *CandlepinClient) reportHostGuestMapping(ctx context.Context, data hostGuestMappingRequest) (string, error) {
	url := fmt.Sprintf("%s/hypervisors/%s?reporter_id=%s",
		c.baseURL, url.PathEscape(c.orgID), url.QueryEscape(c.reporterID))

	body, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("cannot marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "text/plain")
	return c.submitJob(req)
}

func (c *CandlepinClient) submitJob(req *http.Request) (string, error) {
	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if err := c.handleHTTPError(resp); err != nil {
		return "", err
	}

	var job jobResponse
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return "", fmt.Errorf("cannot parse job response: %w", err)
	}
	if strings.TrimSpace(job.ID) == "" {
		return "", fmt.Errorf("job response is missing an id")
	}

	return job.ID, nil
}

func (c *CandlepinClient) waitForJobCompletion(ctx context.Context, jobID string) error {
	ctx, cancel := context.WithTimeoutCause(ctx, jobPollTimeout,
		fmt.Errorf("job polling timeout after %v: %w", jobPollTimeout, context.DeadlineExceeded))
	defer cancel()

	url := fmt.Sprintf("%s/jobs/%s", c.baseURL, url.PathEscape(jobID))

	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-timer.C:
			timer.Reset(jobPollInterval)
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}
			c.setHeaders(req)

			resp, err := c.httpClient.Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return context.Cause(ctx)
				}
				slog.Debug("job poll request failed, retrying", "err", err)
				continue
			}

			if err := c.handleHTTPError(resp); err != nil {
				resp.Body.Close()
				return err
			}
			var job jobResponse
			err = json.NewDecoder(resp.Body).Decode(&job)
			resp.Body.Close()
			if ctx.Err() != nil {
				return context.Cause(ctx)
			}

			if err != nil {
				slog.Debug("cannot parse job response, retrying", "err", err)
				continue
			}

			slog.Debug("job status", "state", job.State)

			finished, err := evaluateJobState(jobID, job.State)
			if err != nil {
				return err
			}
			if finished {
				return nil
			}
		}
	}
}

func evaluateJobState(jobID, state string) (bool, error) {
	switch state {
	case "FINISHED":
		return true, nil
	case "FAILED", "CANCELED":
		return false, fmt.Errorf("job %s: %s", jobID, state)
	case "CREATED", "RUNNING":
		return false, nil
	default:
		slog.Warn("unknown job state", "state", state)
		return false, nil
	}
}

func (c *CandlepinClient) handleHTTPError(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	slog.Debug("error response body", "status", resp.StatusCode, "body", string(body))

	switch resp.StatusCode {
	case http.StatusGone:
		return fmt.Errorf("consumer certificate invalid (HTTP 410 Gone) - system may need re-registration")
	case http.StatusTooManyRequests:
		retryAfter := resp.Header.Get("Retry-After")
		return fmt.Errorf("rate limited (HTTP 429), retry after: %s", retryAfter)
	default:
		body = body[:min(len(body), 1024)]
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
}

func (c *CandlepinClient) setHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("X-Correlation-ID", c.correlationID)
	req.Header.Set("Accept", "application/json")
}
