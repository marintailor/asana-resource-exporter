// Package main provides the main entry point for the Asana resource exporter application.
// It handles initialization, configuration, logging setup, and graceful shutdown.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	app, err := newApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	if err := app.run(); err != nil {
		app.log.Error("application error",
			slog.String("error", err.Error()))
		os.Exit(1)
	}
	app.log.Info("application completed successfully")
}

func (a *app) run() error {
	a.log.Debug("app started")

	a.done = make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		a.log.Info("received signal, initiating shutdown", slog.String("signal", sig.String()))
		a.cancel()
	}()

	interval, err := a.parseInterval()
	if err != nil {
		return err
	}

	if interval > 0 {
		a.log.Debug("run with interval", slog.String("interval", interval.String()))
		return a.runWithInterval(ctx, interval)
	}

	a.log.Debug("run once", slog.String("interval", interval.String()))
	return a.runOnce(ctx)
}

// parseInterval converts the configured interval string into a time.Duration.
// It validates that the interval is at least 1 second if specified.
// Returns 0 duration if no interval was configured.
func (a *app) parseInterval() (time.Duration, error) {
	if a.cfg.interval == "" {
		return 0, nil
	}

	interval, err := time.ParseDuration(a.cfg.interval)
	if err != nil {
		return 0, fmt.Errorf("invalid interval format: %w", err)
	}
	if interval < time.Second {
		return 0, fmt.Errorf("interval must be at least 1 second")
	}

	a.log.Info("running with interval", slog.String("interval", interval.String()))
	return interval, nil
}

// runWithInterval executes export operations periodically at the specified interval.
// It manages concurrent exports using goroutines and aggregates errors.
// The operation continues until the context is cancelled or a fatal error occurs.
func (a *app) runWithInterval(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	errCh := make(chan error, 1)
	var errs []error

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		if err := a.export(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.log.Error("export error", slog.String("error", err.Error()))
			errCh <- err
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return a.finish(ctx, errs)
		case <-ticker.C:
			a.log.Debug("starting interval-based export")
			a.wg.Add(1)
			go func() {
				defer a.wg.Done()
				if err := a.export(ctx); err != nil && !errors.Is(err, context.Canceled) {
					a.log.Error("export error", slog.String("error", err.Error()))
					errCh <- err
				}
			}()
		case err := <-errCh:
			errs = append(errs, err)
		}
	}
}

// runOnce performs a single export operation and returns any errors encountered.
// It respects context cancellation for graceful shutdown.
func (a *app) runOnce(ctx context.Context) error {
	var errs []error
	if err := a.export(ctx); err != nil && !errors.Is(err, context.Canceled) {
		a.log.Error("export error", slog.String("error", err.Error()))
		errs = append(errs, err)
	}
	return a.finish(ctx, errs)
}

// retryAfter parses a Retry-After header value and returns the duration to wait.
// It supports three formats:
// - Duration string (e.g., "30s", "1m")
// - Number of seconds as integer
// - HTTP date format
// If parsing fails, it returns the default retry duration.
func (a *app) retryAfter(s string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}

	if seconds, err := strconv.Atoi(s); err == nil {
		return time.Duration(seconds) * time.Second
	}

	if t, err := http.ParseTime(s); err == nil {
		wait := time.Until(t)
		if wait > 0 {
			return wait
		}
	}

	return time.Duration(defaultRetryAfter) * time.Second
}

// finish handles cleanup operations and aggregates errors before shutdown.
// It waits for running operations to complete with a timeout and returns
// any errors encountered during execution.
func (a *app) finish(ctx context.Context, errs []error) error {
	go func() {
		a.wg.Wait()
		close(a.done)
	}()

	a.cleanup()

	if len(errs) > 0 {
		return fmt.Errorf("encountered %d errors during export, first error: %w", len(errs), errs[0])
	}

	if ctx.Err() == context.Canceled {
		a.log.Info("graceful shutdown completed")
		return nil
	}

	a.log.Info("all tasks completed successfully")
	return nil
}

// cleanup performs cleanup operations during shutdown, including closing
// idle connections. It implements a timeout to prevent hanging during cleanup.
func (a *app) cleanup() {
	select {
	case <-a.done:
		a.log.Debug("all operations completed, cleaning up connections")
	case <-time.After(30 * time.Second):
		a.log.Warn("cleanup timeout reached, forcing shutdown")
	}
	a.client.CloseIdleConnections()
}
