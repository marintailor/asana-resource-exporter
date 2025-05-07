// Package main provides the main entry point for the Asana resource extractor application.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/marintailor/asana-resource-exporter/internal"
)

const (
	// API defaults
	defaultEntrypoint string = "https://app.asana.com/api/1.0"
	defaultRateLimit  int    = 150
	defaultRetryAfter int    = 5

	// Logging defaults
	defaultLogFormat string = "text"
	defaultLogOutput string = ""

	// Data defautls
	dataDir     string = "data"
	permissions int    = 0o755
)

// app orchestrates the resource export operations.
type app struct {
	cfg    *config
	log    *slog.Logger
	client *internal.Client
}

// Resource represents an Asana resource with its properties.
type Resource struct {
	GID          string `json:"gid"`
	Name         string `json:"name"`
	ResourceType string `json:"resource_type"`
}

type options struct {
	cfg config
	log logging
}

type config struct {
	entrypoint string
	resource   string
	rate       int
}

type logging struct {
	debug  bool
	format string
	output string
}

func main() {
	app, err := newApp()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to initialize application: %v\n", err)
		os.Exit(1)
	}

	if err := app.run(); err != nil {
		app.log.Error("app run", slog.String("error", err.Error()))
	}

	app.log.Debug("app finished")
}

// newApp creates and configures a new application instance.
func newApp() (*app, error) {
	var a app

	opts, err := newOptions()
	if err != nil {
		return nil, fmt.Errorf("new options: %w", err)
	}

	cfg, err := newConfig(opts)
	if err != nil {
		return nil, fmt.Errorf("new config: %w", err)
	}

	log, err := newLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("new logger: %w", err)
	}

	client, err := internal.NewClient(cfg.rate)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	a.cfg = cfg
	a.log = log
	a.client = client

	// Add signal handling for graceful shutdown

	a.log.Debug("app details",
		slog.String("config", fmt.Sprintf("%+v", opts.cfg)),
		slog.String("logging", fmt.Sprintf("%+v", opts.log)))

	return &a, nil
}

func (a *app) run() error {
	a.log.Debug("app started")
	var wg sync.WaitGroup
	for i := range 200 {
		wg.Add(1)
		a.log.Debug("export", slog.Int("number", i))
		go func() {
			defer wg.Done()
			if err := a.export(context.Background()); err != nil {
				a.log.Error("export", slog.String("error", err.Error()))
			}
		}()
	}
	wg.Wait()

	return nil
}

func newOptions() (options, error) {
	var o options

	flag.StringVar(&o.cfg.entrypoint, "entrypoint", defaultEntrypoint, "Asana API entrypoint")
	flag.IntVar(&o.cfg.rate, "rate", defaultRateLimit, "request rate limit per minute. ex: 10, 150")
	flag.StringVar(&o.cfg.resource, "resource", "", "Asana resource type to be exported. ex: project, user")
	flag.BoolVar(&o.log.debug, "debug", false, "enable debug log messages")
	flag.StringVar(&o.log.format, "log-format", defaultLogFormat, "log message format. ex: json, text")
	flag.StringVar(&o.log.output, "log-output", defaultLogOutput, "path to file where to store log message. ex: app.log. If omitted then log will be sent to STDOUT")
	flag.Parse()

	return o, nil
}

func newConfig(opts options) (*config, error) {
	if opts.cfg.entrypoint == "" {
		return nil, errors.New("entrypoint not provided")
	}
	// Additional validation, check if entrypoint is a valid URL

	return &opts.cfg, nil
}

// validLogFormat returns whether the provided log format is supported.
func validLogFormat(format string) bool {
	return format == "json" || format == "text"
}

// newLogger creates a new structured logger with the given options. The logger writes to either
// a file specified by opts.log.output or stdout if no output file is provided. It supports
// both JSON and text formats.
func newLogger(opts options) (*slog.Logger, error) {
	if !validLogFormat(opts.log.format) {
		return nil, fmt.Errorf("unsupported log format: %s", opts.log.format)
	}

	var output *os.File
	switch {
	case opts.log.output != "":
		file, err := os.OpenFile(opts.log.output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("log output: %w", err)
		}
		output = file
	default:
		output = os.Stdout
	}

	logOpts := slog.HandlerOptions{}
	if opts.log.debug {
		logOpts.Level = slog.LevelDebug
	}

	switch opts.log.format {
	case "json":
		return slog.New(slog.NewJSONHandler(output, &logOpts)), nil
	default:
		return slog.New(slog.NewTextHandler(output, &logOpts)), nil
	}
}

func (a *app) export(ctx context.Context) error {
	data, err := a.fetchData(ctx)
	if err != nil {
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
		a.storeResource(rc)
	}

	return nil
}

func (a *app) fetchData(ctx context.Context) ([]byte, error) {
	for {
		endpoint := fmt.Sprintf("%s/%ss", a.cfg.entrypoint, a.cfg.resource)
		resp, err := a.client.Request(ctx, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("make request: %w", err)
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			ra := resp.Header.Get("Retry-After")
			if ra != "" {
				wait := a.retryAfter(ra)
				a.log.Warn("too many requests",
					slog.String("retry_after", wait.String()),
					slog.Int("default_wait", defaultRetryAfter))
				time.Sleep(wait)
				continue
			}
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}

		defer func() {
			if err := resp.Body.Close(); err != nil {
				a.log.Error("close request body", slog.String("error", err.Error()))
			}
		}()

		return data, nil
	}
}

func (a *app) resourceDir() error {
	dir, err := os.Stat("data/" + a.cfg.resource)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			a.log.Warn("destination directory does not exist",
				slog.String("path", "data/"+a.cfg.resource),
			)
			if err := os.MkdirAll("data/"+a.cfg.resource, 0o755); err != nil {
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
