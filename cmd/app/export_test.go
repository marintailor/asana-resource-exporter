package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAppExport(t *testing.T) {
	app := &app{
		cfg: &config{
			entrypoint: "example.com",
			resource:   "project",
			rate:       60,
		},
		log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
	}

	buf := new(bytes.Buffer)

	_ = json.NewEncoder(buf).Encode(map[string][]Resource{
		"data": {
			{GID: "1", Name: "Test1", ResourceType: "project"},
			{GID: "2", Name: "Test2", ResourceType: "project"},
		},
	})

	ctx := context.Background()

	rcDir := filepath.Join(t.TempDir(), "project")

	if err := app.resourceDir(rcDir); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	if err := app.export(ctx, buf.Bytes(), rcDir); err != nil {
		t.Errorf("export() error = %v", err)
	}
}

func TestAppExportCancellation(t *testing.T) {
	app := &app{
		cfg: &config{
			entrypoint: "example.com",
			resource:   "project",
			rate:       60,
		},
		log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	buf := new(bytes.Buffer)

	_ = json.NewEncoder(buf).Encode(map[string][]Resource{
		"data": {
			{GID: "1", Name: "Test1", ResourceType: "project"},
			{GID: "2", Name: "Test2", ResourceType: "project"},
		},
	})

	rcDir := filepath.Join(t.TempDir(), "project")
	err := app.export(ctx, buf.Bytes(), rcDir)
	if err != context.Canceled {
		t.Errorf("Expected context.Canceled error, got %v", err)
	}
}

// func TestAppFetchDataRateLimit(t *testing.T) {
// 	callCount := 0
// 	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		callCount++
// 		if callCount == 1 {
// 			w.Header().Set("Retry-After", "1")
// 			w.WriteHeader(http.StatusTooManyRequests)
// 			return
// 		}
// 		w.Header().Set("Content-Type", "application/json")
// 		json.NewEncoder(w).Encode(map[string][]Resource{
// 			"data": {{GID: "1", Name: "Test", ResourceType: "project"}},
// 		})
// 	}))
// 	defer server.Close()

// 	app := &app{
// 		cfg: &config{
// 			entrypoint: server.URL,
// 			resource:   "project",
// 			rate:       60,
// 		},
// 		log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
// 	}

// 	ctx := context.Background()
// 	data, err := app.fetchData(ctx)
// 	if err != nil {
// 		t.Errorf("fetchData() error = %v", err)
// 	}
// 	if len(data) == 0 {
// 		t.Error("fetchData() returned empty data")
// 	}
// 	if callCount != 2 {
// 		t.Errorf("Expected 2 API calls, got %d", callCount)
// 	}
// }

func TestAppResources(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    int
		wantErr bool
	}{
		{
			name:    "valid data",
			data:    `{"data": [{"gid": "1", "name": "Test1", "resource_type": "project"}]}`,
			want:    1,
			wantErr: false,
		},
		{
			name:    "empty data array",
			data:    `{"data": []}`,
			want:    0,
			wantErr: false,
		},
		{
			name:    "invalid json",
			data:    `{"data": [invalid]}`,
			want:    0,
			wantErr: true,
		},
	}

	app := &app{
		cfg: &config{
			entrypoint: "example.com",
			resource:   "project",
			rate:       60,
		},
		log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := app.resources([]byte(tt.data))
			if (err != nil) != tt.wantErr {
				t.Errorf("resources() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(resources) != tt.want {
				t.Errorf("resources() got %v resources, want %v", len(resources), tt.want)
			}
		})
	}
}

func TestAppResourceDir(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		resource string
		setup    func() error
		wantErr  bool
	}{
		{
			name:     "create new directory",
			resource: "project",
			setup:    func() error { return nil },
			wantErr:  false,
		},
		{
			name:     "directory already exists",
			resource: "user",
			setup: func() error {
				return os.MkdirAll(filepath.Join(tmpDir, "user"), 0755)
			},
			wantErr: false,
		},
		{
			name:     "path exists but is file",
			resource: "task",
			setup: func() error {
				return os.WriteFile(filepath.Join(tmpDir, "task"), []byte("test"), 0644)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.setup(); err != nil {
				t.Fatalf("Setup failed: %v", err)
			}

			app := &app{
				cfg: &config{resource: tt.resource},
				log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
			}

			err := app.resourceDir(tmpDir + "/" + tt.resource)
			if (err != nil) != tt.wantErr {
				t.Errorf("resourceDir() error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr {
				if _, err := os.Stat(filepath.Join(tmpDir, tt.resource)); os.IsNotExist(err) {
					t.Error("Expected directory to exist")
				}
			}
		})
	}
}

func TestAppStoreResource(t *testing.T) {
	rcDir := filepath.Join(t.TempDir(), "project")

	resource := Resource{
		GID:          "1",
		Name:         "TestProject",
		ResourceType: "project",
	}

	app := &app{
		cfg: &config{
			entrypoint: "example.com",
			resource:   "project",
			rate:       60,
		},
		log: slog.New(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{})),
	}

	if err := app.resourceDir(rcDir); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	filename := rcDir + "/" + fmt.Sprintf("%s_%s.json", resource.Name, time.Now().Format("20060102150405"))

	if err := app.storeResource(resource, filename); err != nil {
		t.Fatalf("Failed to store resource: %v", err)
	}

	files, err := os.ReadDir(rcDir)
	if err != nil {
		t.Fatalf("Failed to read directory: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}

	content, err := os.ReadFile(filepath.Join(rcDir, files[0].Name()))
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var stored Resource
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatalf("Failed to unmarshal stored resource: %v", err)
	}

	if stored.GID != resource.GID || stored.Name != resource.Name || stored.ResourceType != resource.ResourceType {
		t.Error("Stored resource does not match original")
	}
}
