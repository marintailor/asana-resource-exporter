// Package main provides the main entry point for the Asana resource extractor application.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func (a *app) runOnce(ctx context.Context) error {
	var errs []error
	if err := a.export(ctx); err != nil && !errors.Is(err, context.Canceled) {
		a.log.Error("export error", slog.String("error", err.Error()))
		errs = append(errs, err)
	}
	return a.finish(ctx, errs)
}

func (a *app) export(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	data, err := a.fetchData(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("fetch data: %w", err)
	}

	resources, err := a.resources(data)
	if err != nil {
		return fmt.Errorf("retrieve resources: %w", err)
	}

	if err := a.resourceDir(); err != nil {
		return fmt.Errorf("resource directory: %w", err)
	}

	for _, rc := range resources {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			a.storeResource(rc)
		}
	}

	a.log.Debug("finished iterating resources")

	return nil
}

func (a *app) fetchData(ctx context.Context) ([]byte, error) {
	a.log.Debug("fetch data")
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		endpoint := fmt.Sprintf("%s/%ss", a.cfg.entrypoint, a.cfg.resource)
		resp, err := a.client.Request(ctx, endpoint, nil)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("make request: %w", err)
		}

		defer func() {
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					a.log.Error("close request body", slog.String("error", err.Error()))
				}
			}
		}()

		a.log.Debug("check response status code")
		if resp.StatusCode == http.StatusTooManyRequests {
			ra := resp.Header.Get("Retry-After")
			if ra != "" {
				wait := a.retryAfter(ra)
				a.log.Warn("too many requests",
					slog.String("retry_after", wait.String()),
					slog.Int("default_wait", defaultRetryAfter))

				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, ctx.Err()
				case <-timer.C:
					continue
				}
			}
		}

		a.log.Debug("read response body")
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		a.log.Debug("finished reading response body")

		return data, nil
	}
}

func (a *app) resourceDir() error {
	dir, err := os.Stat(dataDir + "/" + a.cfg.resource)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			a.log.Warn("destination directory does not exist",
				slog.String("path", "data/"+a.cfg.resource),
			)
			if err := os.MkdirAll("data/"+a.cfg.resource, os.FileMode(permissions)); err != nil {
				return fmt.Errorf("make dir: %w", err)
			}
		default:
			return fmt.Errorf("dir stat: %w", err)
		}
	}

	if !dir.IsDir() {
		return fmt.Errorf("%q not directory", dir.Name())
	}

	return nil
}

func (a *app) resources(d []byte) ([]Resource, error) {
	var output struct {
		Data []Resource `json:"data"`
	}

	if err := json.Unmarshal(d, &output); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}

	return output.Data, nil
}

func (a *app) storeResource(rc Resource) {
	a.log.Debug("store resource")
	filename := fmt.Sprintf("data/%ss/%[1]s_%s_%s.json", a.cfg.resource, rc.Name, time.Now().Format("20060102150405"))
	file, err := os.Create(filename)
	if err != nil {
		a.log.Error("create file", slog.String("error", err.Error()), slog.String("filename", filename))
	}

	defer func() {
		if err := file.Close(); err != nil {
			a.log.Error("close file", slog.String("error", err.Error()), slog.String("filename", filename))
		}
	}()

	encoder := json.NewEncoder(file)
	if err := encoder.Encode(rc); err != nil {
		a.log.Error("encode outout", slog.String("error", err.Error()))
	}
	a.log.Debug("resource stored")
}

// retryAfter parses a Retry-After header value and returns the duration to wait.
// It supports HTTP-date, delta-seconds, and duration formats.
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

// finish handles cleanup and returns appropriate status
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

// cleanup performs necessary cleanup operations during shutdown
func (a *app) cleanup() {
	select {
	case <-a.done:
		a.log.Debug("all operations completed, cleaning up connections")
	case <-time.After(30 * time.Second):
		a.log.Warn("cleanup timeout reached, forcing shutdown")
	}
	a.client.CloseIdleConnections()
}
