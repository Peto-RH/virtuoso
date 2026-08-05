package candlepin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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

	baseURL := fmt.Sprintf("https://%s:%d%s", destConfig.Server, destConfig.Port, destConfig.Prefix)
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

func (c *CandlepinClient) Send(ctx context.Context, hypervisor *report.Hypervisor) error {
	slog.Debug("sending hypervisor data", "guests", len(hypervisor.Guests))

	requestBody := newHostGuestMappingRequest(hypervisor)

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

func WriteHostGuestMapping(w io.Writer, hypervisor *report.Hypervisor) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(newHostGuestMappingRequest(hypervisor)); err != nil {
		return fmt.Errorf("failed to encode host-guest mapping: %w", err)
	}
	return nil
}

func (c *CandlepinClient) Ping(ctx context.Context) error {
	slog.Debug("testing Candlepin connection and org access")

	url := fmt.Sprintf("%s/hypervisors/%s/heartbeat?reporter_id=status_test", c.baseURL, c.orgID)
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
	url := fmt.Sprintf("%s/hypervisors/%s?reporter_id=%s", c.baseURL, c.orgID, c.reporterID)

	body, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("cannot marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "text/plain")

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

	return job.ID, nil
}

func (c *CandlepinClient) waitForJobCompletion(ctx context.Context, jobID string) error {
	url := fmt.Sprintf("%s/jobs/%s", c.baseURL, jobID)

	ticker := time.NewTicker(jobPollInterval)
	defer ticker.Stop()

	timeout := time.After(jobPollTimeout)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("job polling timeout after %v", jobPollTimeout)
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
			if err != nil {
				return err
			}
			c.setHeaders(req)

			resp, err := c.httpClient.Do(req)
			if err != nil {
				slog.Debug("job poll request failed, retrying", "err", err)
				continue
			}

			var job jobResponse
			err = json.NewDecoder(resp.Body).Decode(&job)
			resp.Body.Close()

			if err != nil {
				slog.Debug("cannot parse job response, retrying", "err", err)
				continue
			}

			slog.Debug("job status", "state", job.State)

			switch job.State {
			case "FINISHED":
				return nil
			case "FAILED", "CANCELED":
				return fmt.Errorf("job %s: %s", jobID, job.State)
			case "CREATED", "WAITING", "RUNNING":
				continue
			default:
				slog.Warn("unknown job state", "state", job.State)
			}
		}
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
