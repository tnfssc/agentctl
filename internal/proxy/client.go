// Package proxy talks to a CLIProxyAPI server: health, model list and, with a
// management key, the state of each upstream login.
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

// Login is one upstream account the proxy holds.
type Login struct {
	Name          string    `json:"name"`
	Provider      string    `json:"provider"`
	Email         string    `json:"email"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message"`
	Disabled      bool      `json:"disabled"`
	Unavailable   bool      `json:"unavailable"`
	NextRetry     time.Time `json:"next_retry_after"`
	Quota         struct {
		Exceeded      bool      `json:"exceeded"`
		Reason        string    `json:"reason"`
		NextRecoverAt time.Time `json:"next_recover_at"`
	} `json:"quota"`
}

// Logins lists the proxy's upstream accounts. It needs the management key and
// uses the v0 path, which both v7 and v8 servers serve.
func (c *Client) Logins(ctx context.Context, managementKey string) ([]Login, error) {
	var body struct {
		Files []Login `json:"files"`
	}
	if err := c.get(ctx, "/v0/management/auth-files", managementKey, &body); err != nil {
		return nil, err
	}
	return body.Files, nil
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
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
