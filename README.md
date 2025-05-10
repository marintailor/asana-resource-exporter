# Asana Resource Exporter

A Go application for exporting resources from Asana's API with advanced features like configurable rate limiting, interval-based exports, and structured logging.

## Features

- Export any Asana resource type (projects, users, etc.)
- Configurable rate limiting to respect API constraints
- Interval-based data export with customizable durations
- Graceful shutdown handling
- Structured logging with JSON/text formats
- Automatic retry handling for rate limits (429 responses)
- Data persistence to local files

## Prerequisites

- Go 1.21 or higher
- Asana API token

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
- `-debug` - Enable debug logging (default: false)
- `-log-format` - Log format ["json", "text"] (default: "text")
- `-log-output` - Log output file path (default: stdout)

## Usage

Basic usage to export projects:
```bash
export ASANA_API_TOKEN="your_token_here"
asana-resource-exporter -resource=project
```

Export users every minute with debug logging:
```bash
asana-resource-exporter -resource=user -interval=1m -debug -log-format=json
```

## Data Storage

Exported resources are stored in JSON format under the `data/{resource_type}` directory, with filenames containing the resource name and timestamp.

Example: `data/projects/project_MyProject_20240205143022.json`

## Error Handling

The application includes robust error handling for:
- API rate limits (with automatic retries)
- Network issues
- Invalid configurations
- Resource access problems

All errors are logged with appropriate context and the application exits with a non-zero status code on fatal errors.

## Development

### Project Structure

```
asana-resource-exporter/
├── cmd/
│   └── app/
│       ├── app.go        # Application setup and configuration
│       ├── export.go     # Resource export logic
│       └── main.go       # Entry point
├── internal/
│   ├── client.go         # HTTP client with rate limiting
│   └── validator.go      # URL validation utilities
└── README.md
```

### Building

```bash
go build -o asana-resource-exporter ./cmd/app
```

## License

MIT License - See LICENSE file for details.
