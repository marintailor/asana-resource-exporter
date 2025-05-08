package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/marintailor/asana-resource-exporter/internal"
)

const (
	// API defaults
	defaultEntrypoint string = "https://app.asana.com/api/1.0"
	defaultInterval   string = ""
	defaultRateLimit  int    = 150
	defaultRetryAfter int    = 5

	// Logging defaults
	defaultLogFormat string = "text"
	defaultLogOutput string = ""

	// Data defaults
	dataDir     string = "data"
	permissions int    = 0o755
)

// app orchestrates the resource export operations.
type app struct {
	cfg    *config
	log    *slog.Logger
	client *internal.Client
	cancel context.CancelFunc
	wg     sync.WaitGroup
	done   chan struct{}
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
	interval   string
	resource   string
	rate       int
}

type logging struct {
	debug  bool
	format string
	output string
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

	token, ok := os.LookupEnv("ASANA_API_TOKEN")
	if !ok {
		return nil, errors.New("token not present")
	}

	client, err := internal.NewClient(token, cfg.rate)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}

	a.cfg = cfg
	a.log = log
	a.client = client

	a.log.Debug("app details",
		slog.String("config", fmt.Sprintf("%+v", opts.cfg)),
		slog.String("logging", fmt.Sprintf("%+v", opts.log)))

	return &a, nil
}

func newOptions() (options, error) {
	var o options

	flag.StringVar(&o.cfg.entrypoint, "entrypoint", defaultEntrypoint, "Asana API entrypoint")
	flag.StringVar(&o.cfg.interval, "interval", defaultInterval, "interval duration at which to fetch data; ex: 10s, 1m; default: none")
	flag.IntVar(&o.cfg.rate, "rate", defaultRateLimit, "request rate limit per minute. ex: 10, 150")
	flag.StringVar(&o.cfg.resource, "resource", "", "Asana resource type to be exported. ex: project, user")
	flag.BoolVar(&o.log.debug, "debug", false, "enable debug log messages")
	flag.StringVar(&o.log.format, "log-format", defaultLogFormat, "log message format. ex: json, text")
	flag.StringVar(&o.log.output, "log-output", defaultLogOutput, "path to file where to store log message; ex: relative/path/app.log, /absolute/path/app/log; default: STDOUT")
	flag.Parse()

	return o, nil
}

func newConfig(opts options) (*config, error) {
	if opts.cfg.entrypoint == "" {
		return nil, errors.New("entrypoint not provided")
	}
	if opts.cfg.resource == "" {
		return nil, errors.New("resource type not provided")
	}
	if opts.cfg.rate < 1 {
		return nil, errors.New("rate limit must be positive")
	}

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
