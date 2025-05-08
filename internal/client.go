// Package internal provides internal utilities for the Asana resource exporter.
package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

var (
	ErrInvalidEndpoint = errors.New("invalid endpoint")
	ErrReachedLimit    = errors.New("reached limit")
)

// Client wraps http.Client with Asana API authentication.
type Client struct {
	*http.Client
	token    string
	limiter  *rate.Limiter
	shutdown chan struct{} // Channel to signal shutdown
}

// CloseIdleConnections closes any idle HTTP connections.
func (c *Client) CloseIdleConnections() {
	c.Client.CloseIdleConnections()
}

// NewClient creates a Client instance with provided token t and rate r..
func NewClient(t string, r int) (*Client, error) {
	return &Client{
		Client:   &http.Client{},
		token:    t,
		limiter:  rate.NewLimiter(rate.Limit(r/60), r),
		shutdown: make(chan struct{}),
	}, nil
}

// Request performs an authenticated HTTP GET request to the specified Asana endpoint.
func (c *Client) Request(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	if !validEndpoint(url) {
		return nil, ErrInvalidEndpoint
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.limiter.Wait(ctx); err != nil {
		return nil, fmt.Errorf("rate limit wait: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			if resp != nil {
				_ = resp.Body.Close()
			}
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

// validEndpoint validates if the given string is a valid API endpoint URL.
func validEndpoint(rawURL string) bool {
	if strings.TrimSpace(rawURL) == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	if u.Scheme == "" || u.Host == "" {
		return false
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	if u.Fragment != "" {
		return false
	}

	// Reject URLs with user info
	if u.User != nil {
		return false
	}

	host := strings.ToLower(u.Host)
	if strings.Contains(host, "localhost") ||
		strings.Contains(host, "127.0.0.1") ||
		strings.Contains(host, "::1") {
		return false
	}

	if len(rawURL) > 2048 {
		return false
	}

	return true
}
