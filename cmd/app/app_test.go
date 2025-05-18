package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewApp(t *testing.T) {
	origToken := os.Getenv("ASANA_API_TOKEN")
	defer func() {
		_ = os.Setenv("ASANA_API_TOKEN", origToken)
	}()

	tests := []struct {
		name      string
		args      []string
		token     string
		wantError bool
	}{
		{
			name:      "valid configuration",
			args:      []string{"cmd", "-resource", "project", "-rate", "60"},
			token:     "test-token",
			wantError: false,
		},
		{
			name:      "missing token",
			args:      []string{"cmd", "-resource", "project", "-rate", "60"},
			token:     "",
			wantError: true,
		},
		{
			name:      "invalid rate limit",
			args:      []string{"cmd", "-resource", "project", "-rate", "0"},
			token:     "test-token",
			wantError: true,
		},
		{
			name:      "missing resource",
			args:      []string{"cmd", "-rate", "60"},
			token:     "test-token",
			wantError: true,
		},
		{
			name:      "valid configuration with silent logging",
			args:      []string{"cmd", "-resource", "project", "-rate", "60"},
			token:     "test-token",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			if tt.token != "" {
				_ = os.Setenv("ASANA_API_TOKEN", tt.token)
			} else {
				_ = os.Unsetenv("ASANA_API_TOKEN")
			}

			app, err := newApp(tt.args)
			if (err != nil) != tt.wantError {
				t.Errorf("newApp() error = %v, wantError %v", err, tt.wantError)
				return
			}
			if err == nil {
				if app.cfg == nil {
					t.Error("newApp() config is nil")
				}
				if app.log == nil {
					t.Error("newApp() logger is nil")
				}
				if app.client == nil {
					t.Error("newApp() client is nil")
				}
			}
		})
	}
}

func TestNewOptions(t *testing.T) {

	tests := []struct {
		name    string
		args    []string
		want    options
		wantErr bool
	}{
		{
			name: "default values",
			args: []string{"cmd"},
			want: options{
				cfg: config{
					entrypoint: defaultEntrypoint,
					interval:   defaultInterval,
					rate:       defaultRateLimit,
					resource:   "",
				},
				log: logging{
					debug:  false,
					format: defaultLogFormat,
					output: defaultLogOutput,
				},
			},
			wantErr: false,
		},
		{
			name: "custom values",
			args: []string{
				"cmd",
				"-entrypoint", "https://custom.api.com",
				"-interval", "10s",
				"-rate", "100",
				"-resource", "project",
				"-debug",
				"-log-format", "json",
				"-log-output", "app.log",
			},
			want: options{
				cfg: config{
					entrypoint: "https://custom.api.com",
					interval:   "10s",
					rate:       100,
					resource:   "project",
				},
				log: logging{
					debug:  true,
					format: "json",
					output: "app.log",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			got, err := newOptions(tt.args)
			if (err != nil) != tt.wantErr {
				t.Errorf("newOptions() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got.cfg.entrypoint != tt.want.cfg.entrypoint {
				t.Errorf("newOptions() entrypoint = %v, want %v", got.cfg.entrypoint, tt.want.cfg.entrypoint)
			}
			if got.cfg.interval != tt.want.cfg.interval {
				t.Errorf("newOptions() interval = %v, want %v", got.cfg.interval, tt.want.cfg.interval)
			}
			if got.cfg.rate != tt.want.cfg.rate {
				t.Errorf("newOptions() rate = %v, want %v", got.cfg.rate, tt.want.cfg.rate)
			}
			if got.cfg.resource != tt.want.cfg.resource {
				t.Errorf("newOptions() resource = %v, want %v", got.cfg.resource, tt.want.cfg.resource)
			}
			if got.log.debug != tt.want.log.debug {
				t.Errorf("newOptions() debug = %v, want %v", got.log.debug, tt.want.log.debug)
			}
			if got.log.format != tt.want.log.format {
				t.Errorf("newOptions() format = %v, want %v", got.log.format, tt.want.log.format)
			}
			if got.log.output != tt.want.log.output {
				t.Errorf("newOptions() output = %v, want %v", got.log.output, tt.want.log.output)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{
			name: "valid config",
			opts: options{
				cfg: config{
					entrypoint: defaultEntrypoint,
					resource:   "project",
					rate:       60,
				},
			},
			wantErr: false,
		},
		{
			name: "missing entrypoint",
			opts: options{
				cfg: config{
					resource: "project",
					rate:     60,
				},
			},
			wantErr: true,
		},
		{
			name: "missing resource",
			opts: options{
				cfg: config{
					entrypoint: defaultEntrypoint,
					rate:       60,
				},
			},
			wantErr: true,
		},
		{
			name: "invalid rate",
			opts: options{
				cfg: config{
					entrypoint: defaultEntrypoint,
					resource:   "project",
					rate:       0,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := newConfig(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("newConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil && cfg == nil {
				t.Error("newConfig() returned nil config without error")
			}
		})
	}
}

func TestValidLogFormat(t *testing.T) {
	tests := []struct {
		name   string
		format string
		want   bool
	}{
		{"json format", "json", true},
		{"text format", "text", true},
		{"invalid format", "invalid", false},
		{"empty format", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validLogFormat(tt.format); got != tt.want {
				t.Errorf("validLogFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewLogger(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name    string
		opts    options
		wantErr bool
	}{
		{
			name: "stdout text logger",
			opts: options{
				log: logging{
					format: "text",
				},
			},
			wantErr: false,
		},
		{
			name: "stdout json logger",
			opts: options{
				log: logging{
					format: "json",
				},
			},
			wantErr: false,
		},
		{
			name: "file logger",
			opts: options{
				log: logging{
					format: "text",
					output: filepath.Join(tempDir, "test.log"),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid format",
			opts: options{
				log: logging{
					format: "invalid",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid file path",
			opts: options{
				log: logging{
					format: "text",
					output: "/nonexistent/directory/test.log",
				},
			},
			wantErr: true,
		},
		{
			name: "silent logger",
			opts: options{
				log: logging{
					format: "text",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := newLogger(tt.opts)
			if (err != nil) != tt.wantErr {
				t.Errorf("newLogger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				if logger == nil {
					t.Error("newLogger() returned nil logger without error")
				} else {
					logger.Info("test message")
				}
			}
		})
	}
}
