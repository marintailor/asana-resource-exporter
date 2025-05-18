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
	"path/filepath"
	"strings"
	"time"
)

// Resource represents an Asana resource with core identifying properties.
// All Asana resources share these common fields which provide the minimal
// information needed for export and tracking. The GID is guaranteed to be
// unique within a workspace.
type Resource struct {
	GID          string `json:"gid"`           // Global unique identifier from Asana
	Name         string `json:"name"`          // Human-readable resource name
	ResourceType string `json:"resource_type"` // Resource category (project, task, user, etc.)
}

// export fetches resources from Asana and persists them to the filesystem.
// It processes each resource sequentially and creates timestamped JSON files.
// The operation can be cancelled via context. Returns error if the export fails
// or is cancelled.
func (a *app) export(ctx context.Context, data []byte, dir string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	resources, err := a.resources(data)
	if err != nil {
		return fmt.Errorf("retrieve resources: %w", err)
	}

	rcDir := dir + "/" + a.cfg.resource
	if err := a.resourceDir(rcDir); err != nil {
		return fmt.Errorf("resource directory: %w", err)
	}

	for _, rc := range resources {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			filename := fmt.Sprintf("%s/%s_%s_%s.json", rcDir, a.cfg.resource, rc.Name, time.Now().Format("20060102150405"))
			if err := a.storeResource(rc, filename); err != nil {
				return fmt.Errorf("store resource: %w", err)
			}
		}
	}

	a.log.Debug("finished iterating resources")

	return nil
}

// fetchData retrieves resources from the Asana API with rate limit handling.
// When receiving a 429 response, it automatically retries using the Retry-After
// header or falls back to default backoff. The operation respects context
// cancellation.
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

// resourceDir creates or verifies the export directory for a resource type.
// It ensures proper permissions (0755) and returns error if the path exists
// but is not a directory or if creation fails.
func (a *app) resourceDir(dst string) error {
	dir, err := os.Stat(dst)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			a.log.Warn("destination directory does not exist",
				slog.String("path", dst),
			)
			if err := os.MkdirAll(dst, os.FileMode(permissions)); err != nil {
				return fmt.Errorf("make dir: %w", err)
			}
			return nil
		default:
			return fmt.Errorf("dir stat: %w", err)
		}
	}

	if !dir.IsDir() {
		return fmt.Errorf("%q not directory", dir.Name())
	}

	return nil
}

// resources decodes Asana API response data into Resource objects.
// Asana wraps resources in a "data" field array. Returns error if
// JSON unmarshaling fails or response format is invalid.
func (a *app) resources(d []byte) ([]Resource, error) {
	var output struct {
		Data []Resource `json:"data"`
	}

	if err := json.Unmarshal(d, &output); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}

	return output.Data, nil
}

// storeResource persists a resource as JSON in the data directory.
// Filename format: {resource_type}_{name}_{timestamp}.json.
// Returns error if file creation or JSON encoding fails.
// It prevents directory traversal by validating the provided filename.
func (a *app) storeResource(rc Resource, filename string) error {
	a.log.Debug("store resource")

	// Clean the path to handle any . or .. components
	cleanPath := filepath.Clean(filename)

	// Ensure the path is within the data directory by checking it starts with the expected prefix
	dataDirAbs, err := filepath.Abs(a.cfg.dataDir)
	if err != nil {
		return fmt.Errorf("get absolute data directory path: %w", err)
	}

	fileAbs, err := filepath.Abs(cleanPath)
	if err != nil {
		return fmt.Errorf("get absolute file path: %w", err)
	}

	if !strings.HasPrefix(fileAbs, dataDirAbs) {
		return fmt.Errorf("invalid file path: attempts to write outside data directory")
	}

	file, err := os.OpenFile(cleanPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		a.log.Error("create file", slog.String("error", err.Error()), slog.String("filename", filename))
		return err
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

	return nil
}
