package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/marintailor/asana-resource-exporter/internal"
)

func TestAppParseInterval(t *testing.T) {
	tests := []struct {
		name       string
		interval   string
		want       time.Duration
		wantErr    bool
		errMessage string
	}{
		{
			name:     "empty interval",
			interval: "",
			want:     0,
			wantErr:  false,
		},
		{
			name:     "valid interval seconds",
			interval: "5s",
			want:     5 * time.Second,
			wantErr:  false,
		},
		{
			name:     "valid interval minutes",
			interval: "2m",
			want:     2 * time.Minute,
			wantErr:  false,
		},
		{
			name:       "invalid format",
			interval:   "invalid",
			want:       0,
			wantErr:    true,
			errMessage: "invalid interval format",
		},
		{
			name:       "too short interval",
			interval:   "500ms",
			want:       0,
			wantErr:    true,
			errMessage: "interval must be at least 1 second",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &app{
				cfg: &config{interval: tt.interval},
				log: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{})),
			}

			got, err := app.parseInterval()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInterval() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && !strings.Contains(err.Error(), tt.errMessage) {
				t.Errorf("parseInterval() error message = %v, want %v", err, tt.errMessage)
				return
			}
			if got != tt.want {
				t.Errorf("parseInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}

// func TestAppRunWithInterval(t *testing.T) {
// 	tests := []struct {
// 		name       string
// 		interval   time.Duration
// 		sleep      time.Duration
// 		wantRuns   int
// 		wantErr    bool
// 		errMessage string
// 	}{
// 		{
// 			name:     "normal operation - multiple runs",
// 			interval: time.Second,
// 			sleep:    2100 * time.Millisecond,
// 			wantRuns: 2,
// 			wantErr:  false,
// 		},
// 		{
// 			name:     "immediate cancellation",
// 			interval: time.Second,
// 			sleep:    10 * time.Millisecond,
// 			wantRuns: 1,
// 			wantErr:  false,
// 		},
// 		{
// 			name:     "zero interval",
// 			interval: 0,
// 			sleep:    time.Second,
// 			wantRuns: 1,
// 			wantErr:  false,
// 		},
// 		{
// 			name:       "negative interval",
// 			interval:   -time.Second,
// 			sleep:      time.Second,
// 			wantRuns:   0,
// 			wantErr:    true,
// 			errMessage: "negative interval",
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			runCount := 0
// 			app := &app{
// 				cfg: &config{
// 					resource: "project",
// 					interval: tt.interval.String(),
// 					rate:     60,
// 				},
// 				log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
// 			}

// 			ctx, cancel := context.WithCancel(context.Background())
// 			go func() {
// 				time.Sleep(tt.sleep)
// 				cancel()
// 			}()

// 			err := app.runWithInterval(ctx, tt.interval)

// 			if tt.wantErr && err == nil {
// 				t.Error("runWithInterval() expected error but got nil")
// 			}
// 			if !tt.wantErr && err != nil {
// 				t.Errorf("runWithInterval() unexpected error: %v", err)
// 			}
// 			if tt.errMessage != "" && err != nil && !strings.Contains(err.Error(), tt.errMessage) {
// 				t.Errorf("runWithInterval() error = %v, want error containing %v", err, tt.errMessage)
// 			}

// 			if runCount != tt.wantRuns {
// 				t.Errorf("runWithInterval() executed %d times, want %d", runCount, tt.wantRuns)
// 			}
// 		})
// 	}
// }

// func TestAppRunOnce(t *testing.T) {
// 	tests := []struct {
// 		name    string
// 		ctx     context.Context
// 		wantErr bool
// 	}{
// 		{
// 			name:    "normal operation",
// 			ctx:     context.Background(),
// 			wantErr: false,
// 		},
// 		{
// 			name: "cancelled context",
// 			ctx: func() context.Context {
// 				ctx, cancel := context.WithCancel(context.Background())
// 				cancel()
// 				return ctx
// 			}(),
// 			wantErr: false,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			app := &app{
// 				cfg: &config{
// 					resource: "project",
// 					rate:     60,
// 				},
// 				log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
// 			}

// 			err := app.runOnce(tt.ctx)
// 			if (err != nil) != tt.wantErr {
// 				t.Errorf("runOnce() error = %v, wantErr %v", err, tt.wantErr)
// 			}
// 		})
// 	}
// }

func TestAppRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  time.Duration
	}{
		{
			name:  "duration string",
			input: "30s",
			want:  30 * time.Second,
		},
		{
			name:  "seconds as integer",
			input: "60",
			want:  60 * time.Second,
		},
		// {
		// 	name:  "HTTP date format",
		// 	input: time.Now().UTC().Add(2 * time.Minute).Format(http.TimeFormat),
		// 	want:  2 * time.Minute,
		// },
		{
			name:  "empty input",
			input: "",
			want:  time.Duration(defaultRetryAfter) * time.Second,
		},
		{
			name:  "invalid input",
			input: "invalid",
			want:  time.Duration(defaultRetryAfter) * time.Second,
		},
		{
			name:  "past date",
			input: time.Now().UTC().Add(-2 * time.Minute).Format(http.TimeFormat),
			want:  time.Duration(defaultRetryAfter) * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &app{}

			got := app.retryAfter(tt.input)

			if got != tt.want {
				t.Errorf("retryAfter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAppFinish(t *testing.T) {
	tests := []struct {
		name    string
		errs    []error
		ctx     context.Context
		wantErr bool
	}{
		{
			name:    "no errors",
			errs:    nil,
			ctx:     context.Background(),
			wantErr: false,
		},
		{
			name:    "with errors",
			errs:    []error{errors.New("test error")},
			ctx:     context.Background(),
			wantErr: true,
		},
		{
			name: "cancelled context",
			errs: nil,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		client, _ := internal.NewClient("token", 1)
		t.Run(tt.name, func(t *testing.T) {
			app := &app{
				done:   make(chan struct{}),
				log:    slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
				client: client,
			}

			err := app.finish(tt.ctx, tt.errs)
			if (err != nil) != tt.wantErr {
				t.Errorf("finish() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAppCleanup(t *testing.T) {
	tests := []struct {
		name      string
		setupDone bool
		sleepTime time.Duration
		wantLog   string
	}{
		// {
		// 	name:      "normal cleanup",
		// 	setupDone: true,
		// 	sleepTime: 0,
		// 	wantLog:   "all operations completed",
		// },
		{
			name:      "timeout cleanup",
			setupDone: false,
			sleepTime: 31 * time.Second,
			wantLog:   "cleanup timeout",
		},
	}

	for _, tt := range tests {
		client, _ := internal.NewClient("token", 1)
		t.Run(tt.name, func(t *testing.T) {
			app := &app{
				done:   make(chan struct{}),
				log:    slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
				client: client,
			}

			if tt.setupDone {
				close(app.done)
			}

			done := make(chan struct{})
			go func() {
				app.cleanup()
				close(done)
			}()

			select {
			case <-done:
			case <-time.After(tt.sleepTime):
				t.Error("cleanup() did not complete in time")
			}
		})
	}
}
