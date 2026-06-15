package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ingestPath is the path on the telemetry endpoint that accepts Reports.
const ingestPath = "/v1/telemetry"

// Client posts Reports to a telemetry endpoint over HTTPS.
type Client struct {
	endpoint   string
	httpClient *http.Client
}

// NewClient builds a Client. The endpoint may be either a bare base URL
// (e.g. https://telemetry.superphenix.net) or the full ingest URL; the
// ingest path is appended only when it is missing.
func NewClient(endpoint string) *Client {
	return &Client{
		endpoint: normalizeEndpoint(endpoint),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Push serialises the report and POSTs it to the endpoint.
func (c *Client) Push(ctx context.Context, report Report) error {
	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("marshal telemetry report: %w", err)
	}

	if len(body) > MaxBodyBytes {
		return fmt.Errorf("telemetry report exceeds %d bytes", MaxBodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build telemetry request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post telemetry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("telemetry endpoint returned %s", resp.Status)
	}
	return nil
}

// normalizeEndpoint normalizes the telemetry endpoint URL to add the ingest path if it is missing.
func normalizeEndpoint(endpoint string) string {
	endpoint = strings.TrimRight(endpoint, "/")
	if strings.HasSuffix(endpoint, ingestPath) {
		return endpoint
	}
	return endpoint + ingestPath
}
