// Package proxy talks to a CLIProxyAPI server: health and model lists.
package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client calls one proxy.
type Client struct {
	BaseURL string // without a trailing /v1
	HTTP    *http.Client
}

// New normalizes rawURL ("https://host:8317/v1/" and "https://host:8317" are
// the same proxy) and returns a client with a short timeout.
func New(rawURL string) (*Client, error) {
	base, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	return &Client{BaseURL: base, HTTP: &http.Client{Timeout: 15 * time.Second}}, nil
}

// NormalizeURL validates a proxy URL and strips a trailing /v1 and slash.
func NormalizeURL(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("proxy url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" || u.Host == "" {
		return "", fmt.Errorf("proxy url %q must look like https://host:port", rawURL)
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/v1")
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

// Model is one entry of /v1/models.
type Model struct {
	ID      string `json:"id"`
	OwnedBy string `json:"owned_by"`
}

// Health checks the unauthenticated /healthz endpoint.
func (c *Client) Health(ctx context.Context) error {
	var body struct {
		Status string `json:"status"`
	}
	if err := c.get(ctx, "/healthz", "", &body); err != nil {
		return err
	}
	if body.Status != "ok" {
		return fmt.Errorf("healthz status %q", body.Status)
	}
	return nil
}

// Models lists the models the client key can reach, sorted by id.
func (c *Client) Models(ctx context.Context, key string) ([]Model, error) {
	var body struct {
		Data []Model `json:"data"`
	}
	if err := c.get(ctx, "/v1/models", key, &body); err != nil {
		return nil, err
	}
	sort.Slice(body.Data, func(i, j int) bool { return body.Data[i].ID < body.Data[j].ID })
	return body.Data, nil
}

// ModelsAsClaudeCode lists models the way Claude Code's gateway discovery
// asks for them. CLIProxyAPI answers Claude clients in Anthropic format and,
// unless configured otherwise, renames non-Claude models to Claude-looking
// ids; comparing with Models reveals that.
func (c *Client) ModelsAsClaudeCode(ctx context.Context, key string) ([]string, error) {
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	headers := map[string]string{
		"anthropic-version": "2023-06-01",
		"x-api-key":         key,
		"User-Agent":        "claude-cli/2.1.310 (external, cli)",
	}
	if err := c.do(ctx, "/v1/models?limit=1000", key, headers, &body); err != nil {
		return nil, err
	}
	ids := make([]string, len(body.Data))
	for i, m := range body.Data {
		ids[i] = m.ID
	}
	return ids, nil
}

// HTTPError is a non-2xx response.
type HTTPError struct {
	Path   string
	Status int
	Body   string
}

func (e *HTTPError) Error() string {
	msg := fmt.Sprintf("%s: HTTP %d", e.Path, e.Status)
	if e.Body != "" {
		msg += ": " + e.Body
	}
	return msg
}

// IsUnauthorized reports whether err is a 401 or 403 from the proxy.
func IsUnauthorized(err error) bool {
	var he *HTTPError
	return errors.As(err, &he) && (he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden)
}

func (c *Client) get(ctx context.Context, path, bearer string, out any) error {
	return c.do(ctx, path, bearer, nil, out)
}

func (c *Client) do(ctx context.Context, path, bearer string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode/100 != 2 {
		snippet := strings.TrimSpace(string(body))
		if len(snippet) > 200 {
			snippet = snippet[:200] + "…"
		}
		return &HTTPError{Path: path, Status: res.StatusCode, Body: snippet}
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: unexpected response: %w", path, err)
	}
	return nil
}
