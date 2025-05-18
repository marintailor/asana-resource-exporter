// Package internal provides internal utilities for the Asana resource exporter.
// It implements HTTP client management, rate limiting, and API authentication
// mechanisms for interacting with the Asana API.
package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"golang.org/x/time/rate"
)

var (
	ErrInvalidEndpoint = errors.New("invalid endpoint")
	ErrReachedLimit    = errors.New("reached limit")
)

// Client wraps http.Client to provide Asana API authentication and rate limiting.
// It ensures requests respect API rate limits and provides clean shutdown functionality.
type Client struct {
	*http.Client               // Embedded HTTP client for making HTTP requests
	token        string        // Asana personal access token for authentication
	limiter      *rate.Limiter // Rate limiter to control API request frequency
	shutdown     chan struct{} // Channel for coordinating graceful shutdown
}

// CloseIdleConnections closes any idle connections held by the underlying HTTP client.
// It should be called during cleanup to ensure proper resource release.
func (c *Client) CloseIdleConnections() {
	c.Client.CloseIdleConnections()
}

// NewClient creates a new Client with the specified API token and rate limit.
// The rate parameter defines the maximum number of requests allowed per minute.
// It returns an error if initialization fails.
func NewClient(t string, r int) (*Client, error) {
	return &Client{
		Client:   &http.Client{},
		token:    t,
		limiter:  rate.NewLimiter(rate.Limit(r/60), r),
		shutdown: make(chan struct{}),
	}, nil
}

// Request performs an authenticated HTTP GET request to the specified Asana endpoint.
// It enforces rate limiting, handles request timeouts, and adds authentication headers.
// The request respects context cancellation and returns the raw HTTP response.
// If the request fails or is cancelled, returns an error.
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
// It enforces URL format rules including:
// - Proper URL structure and non-empty scheme/host
// - HTTP/HTTPS schemes only
// - No URL fragments or user info
// - Maximum URL length of 2048 characters
// Returns true if URL is valid, false otherwise.
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

	if u.User != nil {
		return false
	}

	// host := strings.ToLower(u.Host)
	// if strings.Contains(host, "localhost") ||
	// 	strings.Contains(host, "127.0.0.1") ||
	// 	strings.Contains(host, "::1") {
	// 	return false
	// }

	if len(rawURL) > 2048 {
		return false
	}

	return true
}
