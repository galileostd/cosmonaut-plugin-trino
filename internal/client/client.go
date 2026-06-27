// Package client implements a minimal Trino HTTP API client.
// It knows nothing about Cosmonaut — it only speaks to Trino.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	defaultTimeout = 10 * time.Second
	trinoUserAgent = "cosmonaut-plugin-trino/v0.1.0"
)

// Client is a minimal Trino HTTP API client.
type Client struct {
	endpoint string
	http     *http.Client
	user     string
}

// New creates a new Trino client.
// endpoint should be the base URL, e.g. "http://trino:8080".
// user is the Trino user to impersonate (required by Trino protocol).
func New(endpoint, user string) *Client {
	if user == "" {
		user = "cosmonaut"
	}
	return &Client{
		endpoint: endpoint,
		user:     user,
		http:     &http.Client{Timeout: defaultTimeout},
	}
}

// InfoResponse is the response from GET /v1/info.
type InfoResponse struct {
	NodeVersion struct {
		Version string `json:"version"`
	} `json:"nodeVersion"`
	Environment string `json:"environment"`
	Starting    bool   `json:"starting"`
	Uptime      string `json:"uptime"`
}

// Info calls GET /v1/info and returns cluster information.
// Used for health checks.
func (c *Client) Info(ctx context.Context) (*InfoResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.endpoint+"/v1/info", nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calling /v1/info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from /v1/info: %d", resp.StatusCode)
	}

	var info InfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding /v1/info response: %w", err)
	}

	return &info, nil
}

// QueryResponse is the initial response from POST /v1/statement.
type QueryResponse struct {
	ID      string `json:"id"`
	InfoURI string `json:"infoUri"`
	Stats   struct {
		State string `json:"state"`
	} `json:"stats"`
	Error *QueryError `json:"error,omitempty"`
}

// QueryError represents a Trino query error.
type QueryError struct {
	Message  string `json:"message"`
	SQLState string `json:"sqlState"`
	ErrorCode int   `json:"errorCode"`
}

// SubmitQuery submits a SQL query to Trino and returns the initial response.
// The query is submitted asynchronously — use the returned query ID to track it.
func (c *Client) SubmitQuery(ctx context.Context, sql string) (*QueryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.endpoint+"/v1/statement",
		stringReader(sql),
	)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(req)
	req.Header.Set("Content-Type", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("submitting query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from /v1/statement: %d", resp.StatusCode)
	}

	var qr QueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("decoding query response: %w", err)
	}

	if qr.Error != nil {
		return nil, fmt.Errorf("query error: %s (code %d)", qr.Error.Message, qr.Error.ErrorCode)
	}

	return &qr, nil
}

// QueryStatusResponse is the response from GET /v1/query/{queryId}.
type QueryStatusResponse struct {
	QueryID string `json:"queryId"`
	State   string `json:"state"`
	Self    string `json:"self"`
	Query   string `json:"query"`
	Stats   struct {
		State           string  `json:"state"`
		Queued          bool    `json:"queued"`
		Running         bool    `json:"running"`
		ElapsedTimeMs   float64 `json:"elapsedTimeMillis"`
		TotalRows       int64   `json:"totalRows"`
		ProcessedRows   int64   `json:"processedRows"`
		ProcessedBytes  int64   `json:"processedBytes"`
	} `json:"stats"`
	FailureInfo *struct {
		Message string `json:"message"`
	} `json:"failureInfo,omitempty"`
}

// QueryStatus returns the current status of a running query.
func (c *Client) QueryStatus(ctx context.Context, queryID string) (*QueryStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.endpoint+"/v1/query/"+queryID, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting query status: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		return nil, fmt.Errorf("query %s not found (may have expired)", queryID)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status from /v1/query/%s: %d", queryID, resp.StatusCode)
	}

	var qs QueryStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&qs); err != nil {
		return nil, fmt.Errorf("decoding query status: %w", err)
	}

	return &qs, nil
}

// CancelQuery cancels a running query.
func (c *Client) CancelQuery(ctx context.Context, queryID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.endpoint+"/v1/query/"+queryID, nil)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("canceling query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status canceling query %s: %d", queryID, resp.StatusCode)
	}

	return nil
}

// SchemaResponse represents a Trino schema.
type SchemaResponse struct {
	Name    string `json:"name"`
	Catalog string `json:"catalog"`
}

// TableResponse represents a Trino table.
type TableResponse struct {
	Name    string `json:"name"`
	Schema  string `json:"schema"`
	Catalog string `json:"catalog"`
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("X-Trino-User", c.user)
	req.Header.Set("X-Trino-Source", trinoUserAgent)
	req.Header.Set("User-Agent", trinoUserAgent)
}

// stringReader returns an io.Reader from a string.
func stringReader(s string) *stringReadCloser {
	return &stringReadCloser{data: []byte(s), pos: 0}
}

type stringReadCloser struct {
	data []byte
	pos  int
}

func (s *stringReadCloser) Read(p []byte) (n int, err error) {
	if s.pos >= len(s.data) {
		return 0, fmt.Errorf("EOF")
	}
	n = copy(p, s.data[s.pos:])
	s.pos += n
	return n, nil
}

func (s *stringReadCloser) Close() error { return nil }
