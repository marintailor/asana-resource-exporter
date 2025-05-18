package internal

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		token   string
		rate    int
		wantErr bool
	}{
		{
			name:    "valid client creation",
			token:   "test-token",
			rate:    60,
			wantErr: false,
		},
		{
			name:    "zero rate",
			token:   "test-token",
			rate:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.token, tt.rate)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if client == nil {
				t.Error("NewClient() returned nil client")
				return
			}
			if client.token != tt.token {
				t.Errorf("NewClient() token = %v, want %v", client.token, tt.token)
			}
			if client.limiter == nil {
				t.Error("NewClient() returned nil rate limiter")
			}
		})
	}
}

func TestClient_Request(t *testing.T) {
	tests := []struct {
		name       string
		token      string
		rate       int
		url        string
		body       io.Reader
		setupMock  func() *httptest.Server
		wantErr    bool
		wantStatus int
	}{
		{
			name:  "valid request",
			token: "test-token",
			rate:  60,
			body:  nil,
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer test-token" {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					w.WriteHeader(http.StatusOK)
				}))
			},
			wantErr:    false,
			wantStatus: http.StatusOK,
		},
		// {
		// 	name:  "invalid endpoint",
		// 	token: "test-token",
		// 	rate:  60,
		// 	url:   "invalid-url",
		// 	body:  nil,
		// 	setupMock: func() *httptest.Server {
		// 		return nil
		// 	},
		// 	wantErr: true,
		// },
		{
			name:  "rate limited request",
			token: "test-token",
			rate:  1,
			body:  nil,
			setupMock: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
				}))
			},
			wantErr:    false,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var server *httptest.Server
			if tt.setupMock != nil {
				server = tt.setupMock()
				defer server.Close()
				if tt.url == "" {
					tt.url = server.URL
				}
			}

			client, err := NewClient(tt.token, tt.rate)
			if err != nil {
				t.Fatalf("Failed to create client: %v", err)
			}

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			resp, err := client.Request(ctx, tt.url, tt.body)
			if (err != nil) != tt.wantErr {
				t.Errorf("Client.Request() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				defer func() {
					_ = resp.Body.Close()
				}()
				if resp.StatusCode != tt.wantStatus {
					t.Errorf("Client.Request() status = %v, want %v", resp.StatusCode, tt.wantStatus)
				}
			}
		})
	}
}

func TestValidEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"invalid URL", "not-a-url", false},
		{"missing scheme", "example.com", false},
		{"invalid scheme", "ftp://example.com", false},
		{"valid http", "http://example.com", true},
		{"valid https", "https://example.com", true},
		{"URL with fragment", "https://example.com#fragment", false},
		{"URL with userinfo", "https://user:pass@example.com", false},
		// {"localhost URL", "http://localhost", false},
		// {"loopback IP", "http://127.0.0.1", false},
		// {"IPv6 loopback", "http://[::1]", false},
		{"very long URL", strings.Repeat("a", 2049), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validEndpoint(tt.endpoint); got != tt.want {
				t.Errorf("validEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_CloseIdleConnections(t *testing.T) {
	client, err := NewClient("test-token", 60)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// This is mostly a smoke test since the actual closing of idle connections
	// is handled by the underlying http.Client
	client.CloseIdleConnections()
}

func TestClient_RequestRateLimit(t *testing.T) {
	client, err := NewClient("test-token", 2) // 2 requests per minute
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx := context.Background()

	// First request should go through immediately
	resp1, err := client.Request(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("First request failed: %v", err)
	}
	_ = resp1.Body.Close()

	// Second request should also go through
	resp2, err := client.Request(ctx, server.URL, nil)
	if err != nil {
		t.Fatalf("Second request failed: %v", err)
	}
	_ = resp2.Body.Close()

	// Create a context with a short timeout
	ctxTimeout, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// Third request should be rate limited and fail due to context timeout
	_, err = client.Request(ctxTimeout, server.URL, nil)
	if err == nil {
		t.Error("Expected rate limit error, got nil")
	}
}

func TestClient_RequestContextCancellation(t *testing.T) {
	client, err := NewClient("test-token", 60)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second) // Simulate a slow response
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err = client.Request(ctx, server.URL, nil)
	if err == nil {
		t.Error("Expected context deadline exceeded error, got nil")
	}
}

func TestClient_RequestInvalidEndpoint(t *testing.T) {
	client, err := NewClient("test-token", 60)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	invalidEndpoints := []string{
		"",
		"   ",
		"not-a-url",
		"ftp://example.com",
		// "http://localhost",
		// "http://127.0.0.1",
		// "http://[::1]",
	}

	for _, endpoint := range invalidEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			_, err := client.Request(context.Background(), endpoint, nil)
			if err == nil {
				t.Errorf("Expected error for invalid endpoint %q, got nil", endpoint)
			}
			if err != ErrInvalidEndpoint {
				t.Errorf("Expected ErrInvalidEndpoint for %q, got %v", endpoint, err)
			}
		})
	}
}
