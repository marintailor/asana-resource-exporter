# Asana Resource Exporter

A high-performance Go application for exporting resources from Asana's API. Features include configurable rate limiting, interval-based exports, structured logging, and robust error handling. Designed for reliability and efficiency when managing large-scale Asana data exports.

**NOTE:** This repository contains a complete and detailed implementation of an application originally developed as part of a live coding assignment during a technical interview process.

## Features

- Export any Asana resource type (projects, users, tasks, etc.)
- Smart rate limiting with automatic backoff
- Configurable export intervals (one-time or periodic)
- Graceful shutdown with cleanup
- Structured logging (JSON/text) with debug support
- Automatic retry with exponential backoff for rate limits
- Local file persistence with timestamp-based naming
- Concurrent export operations
- Context-aware cancellation
- Robust error handling and reporting
- Secure file operations with:
  - Path traversal protection
  - Restrictive file permissions (0600)
  - Safe file handling practices

## Prerequisites

- Go 1.24.2 or higher
- Asana API token with appropriate permissions
- Sufficient disk space for exports
- Network access to Asana API endpoints

## Installation

```bash
go install github.com/marintailor/asana-resource-exporter@latest
```

## Configuration

### Environment Variables

- `ASANA_API_TOKEN` - Your Asana API token (required)

### Command Line Flags

- `-entrypoint` - Asana API endpoint (default: "https://app.asana.com/api/1.0")
- `-interval` - Export interval duration (e.g., "10s", "1m") (default: none)
- `-rate` - Request rate limit per minute (default: 150)
- `-resource` - Resource type to export (e.g., "project", "user") (required)
- `-data-dir` - Directory where exported resources will be stored (default: "data")
- `-debug` - Enable debug logging (default: false)
- `-log-format` - Log format ["json", "text"] (default: "text")
- `-log-output` - Log output file path (default: stdout)

## Usage

Basic usage to export projects:
```bash
export ASANA_API_TOKEN="your_token_here"
asana-resource-exporter -resource=project
```

Export users to custom directory with debug logging:
```bash
asana-resource-exporter -resource=user -data-dir=/exports/asana -debug
```

Export tasks every minute with JSON logging:
```bash
asana-resource-exporter -resource=task -interval=1m -log-format=json
```

## Data Storage

Exported resources are stored in JSON format under the `{data-dir}/{resource_type}` directory (where data-dir defaults to "data" but can be configured), with filenames containing the resource name and timestamp. All files are created with secure permissions (0600) and protected against path traversal attacks.

Example with default data-dir: `data/projects/project_MyProject_20240205143022.json`
Example with custom data-dir: `/exports/data/projects/project_MyProject_20240205143022.json`

The application enforces strict security measures:
- Files are created with 0600 permissions (owner read/write only)
- Paths are validated to prevent directory traversal attacks
- File operations are restricted to the configured data directory

## Error Handling

The application implements comprehensive error handling:

- API Rate Limits
  - Automatic retry with exponential backoff
  - Respects Retry-After headers
  - Configurable maximum retry attempts

- Configuration Errors
  - Invalid API tokens
  - Malformed URLs
  - Invalid rate limits
  - Directory permission issues
  - Path traversal attempts
  - File permission violations

All errors are logged with:
- Detailed error context
- Stack traces in debug mode
- Structured fields for easier parsing
- Non-zero exit codes for fatal errors

## Development

### Project Structure

```
asana-resource-exporter/
├── cmd/
│   └── app/
│       ├── app.go        # Core application setup and DI
│       ├── export.go     # Resource export orchestration
│       └── main.go       # Entry point and signal handling
├── internal/
│   ├── client.go         # Rate-limited HTTP client
├── README.md            # Documentation
└── LICENSE             # MIT License
```

### Building

Development build:
```bash
go build -o asana-resource-exporter ./cmd/app
```

Production build with optimizations:
```bash
go build -ldflags="-s -w" -o asana-resource-exporter ./cmd/app
```

## Roadmap

- [ ] Add support for bulk exports
- [ ] Implement export filters (by date, status, etc.)
- [ ] Enhanced error recovery strategies
- [ ] Add metrics collection and monitoring
- [ ] Add support for parallel workspace exports
- [ ] Implement real-time export streaming

## License

MIT License - See LICENSE file for details.
