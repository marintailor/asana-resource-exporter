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
	"time"
)

// Resource represents an Asana resource with its identifying properties.
// It contains the minimal set of fields common to all Asana resources
// that are necessary for export operations.
type Resource struct {
	GID          string `json:"gid"`           // Unique identifier for the resource
	Name         string `json:"name"`          // Display name of the resource
	ResourceType string `json:"resource_type"` // Type of the resource (e.g., "project", "user")
}

// export performs the main resource export operation. It fetches data from the Asana API,
// processes the resources, and stores them in the filesystem. It respects context cancellation
// for graceful shutdown.
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

// fetchData retrieves resource data from the Asana API, handling rate limiting and retries.
// It automatically retries requests when encountering rate limit responses (429)
// using the Retry-After header value.
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

// resourceDir ensures the data directory for the current resource type exists.
// If the directory doesn't exist, it creates it with appropriate permissions.
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

// resources unmarshals the raw JSON response data into a slice of Resource objects.
// It handles the Asana API response format where resources are nested in a data field.
func (a *app) resources(d []byte) ([]Resource, error) {
	var output struct {
		Data []Resource `json:"data"`
	}

	if err := json.Unmarshal(d, &output); err != nil {
		return nil, fmt.Errorf("unmarshal data: %w", err)
	}

	return output.Data, nil
}

// storeResource writes a single resource to a JSON file in the data directory.
// The filename includes the resource type, name, and current timestamp for uniqueness.
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
