// Package main provides the application core for the Asana resource exporter.
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

	// File system defaults
	permissions int = 0o755
)

// app orchestrates the resource export operations, managing configuration,
// logging, API client, and concurrency control.
type app struct {
	cfg    *config            // Application configuration
	log    *slog.Logger       // Structured logger
	client *internal.Client   // Asana API client
	cancel context.CancelFunc // Context cancellation function
	wg     sync.WaitGroup     // Tracks running goroutines
	done   chan struct{}      // Signals application shutdown
}

// options holds application configuration and logging settings parsed from command-line flags.
type options struct {
	cfg config  // Application configuration settings
	log logging // Logging configuration settings
}

// config defines API-related configuration settings for the application.
type config struct {
	entrypoint string // Asana API endpoint URL
	interval   string // Export interval duration (e.g., "10s", "1m")
	resource   string // Resource type to export (e.g., "project", "user")
	rate       int    // API request rate limit per minute
	dataDir    string // Directory path for storing exported resources
}

// logging defines logging-related configuration settings.
type logging struct {
	debug  bool   // Enable debug logging level
	format string // Log format (json or text)
	output string // Log output destination (file path or stdout)
}

// newApp creates and configures a new application instance with settings from
// command-line flags and environment variables. It initializes logging and the API client.
// Args contain the command-line arguments (e.g., os.Args).
func newApp(args []string) (*app, error) {
	var a app

	opts, err := newOptions(args)
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

// newOptions parses command-line flags into application and logging configuration options.
// Args contain the command-line arguments to parse (e.g., os.Args).
func newOptions(args []string) (options, error) {
	var o options

	flags := flag.NewFlagSet(args[0], flag.ExitOnError)
	flags.StringVar(&o.cfg.entrypoint, "entrypoint", defaultEntrypoint, "Asana API entrypoint")
	flags.StringVar(&o.cfg.interval, "interval", defaultInterval, "interval duration at which to fetch data; ex: 10s, 1m; default: none")
	flags.IntVar(&o.cfg.rate, "rate", defaultRateLimit, "request rate limit per minute. ex: 10, 150")
	flags.StringVar(&o.cfg.resource, "resource", "", "Asana resource type to be exported. ex: project, user")
	flags.BoolVar(&o.log.debug, "debug", false, "enable debug log messages")
	flags.StringVar(&o.log.format, "log-format", defaultLogFormat, "log message format. ex: json, text")
	flags.StringVar(&o.log.output, "log-output", defaultLogOutput, "path to file where to store log message; ex: relative/path/app.log, /absolute/path/app/log; default: STDOUT")
	flags.StringVar(&o.cfg.dataDir, "data-dir", "data", "directory path where exported resources will be stored")

	if err := flags.Parse(args[1:]); err != nil {
		return options{}, fmt.Errorf("parse flags: %w", err)
	}

	return o, nil
}

// newConfig validates and creates a new configuration from the provided options.
// It ensures required fields are set and values are within acceptable ranges.
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

// validLogFormat checks if the provided log format is supported (json or text).
func validLogFormat(format string) bool {
	return format == "json" || format == "text"
}

// newLogger creates a new structured logger with the given options.
// It configures the log level, format (JSON or text), and output destination (file or stdout).
// The logger supports debug level messages when enabled through options.
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
