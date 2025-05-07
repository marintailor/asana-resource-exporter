package internal

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"golang.org/x/time/rate"
)

var (
	ErrInvalidEndpoint = errors.New("invalid endpoint")
	ErrReachedLimit    = errors.New("reached limit")
)

// Client wraps http.Client with Asana API authentication.
type Client struct {
	*http.Client
	token   string
	limiter *rate.Limiter
}

// NewClient creates a Client instance using ASANA_API_TOKEN from environment.
func NewClient(r int) (*Client, error) {
	t, ok := os.LookupEnv("ASANA_API_TOKEN")
	if !ok {
		return nil, errors.New("token not present")
	}

	return &Client{
		Client:  &http.Client{},
		token:   t,
		limiter: rate.NewLimiter(rate.Limit(r/60), r),
		// limiter: rate.NewLimiter(rate.Limit(2), 2),
	}, nil
}

// Request performs an authenticated HTTP GET request to the specified Asana endpoint.
func (c *Client) Request(ctx context.Context, url string, body io.Reader) (*http.Response, error) {
	if !validEndpoint(url) {
		return nil, ErrInvalidEndpoint
	}

	if !c.limiter.Allow() {
		return nil, ErrReachedLimit
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}

	return resp, nil
}

// validEndpoint validates if the given string is a valid API endpoint URL.
func validEndpoint(rawURL string) bool {
	// Reject empty or whitespace-only strings
	if strings.TrimSpace(rawURL) == "" {
		return false
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Must have scheme and host
	if u.Scheme == "" || u.Host == "" {
		return false
	}

	// Only allow HTTP/HTTPS schemes
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}

	// Reject URLs with fragments (#)
	if u.Fragment != "" {
		return false
	}

	// Reject URLs with user info
	if u.User != nil {
		return false
	}

	// Optional: Reject localhost and private IPs for production
	host := strings.ToLower(u.Host)
	if strings.Contains(host, "localhost") ||
		strings.Contains(host, "127.0.0.1") ||
		strings.Contains(host, "::1") {
		return false
	}

	// Optional: Check URL length (adjust limit as needed)
	if len(rawURL) > 2048 {
		return false
	}

	return true
}
