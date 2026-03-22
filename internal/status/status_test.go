package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/jsonfile"
)

// statusResponse is the complete JSON schema from the Atlassian Statuspage API.
// This struct documents every known field and is used in tests with
// DisallowUnknownFields to detect when the API adds new fields.
// Update this struct and testdata/status_*.json when the schema changes.
type statusResponse struct {
	Page struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		TimeZone  string `json:"time_zone"`
		UpdatedAt string `json:"updated_at"`
	} `json:"page"`
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
}

func TestStatusResponseSchema(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/status*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no testdata/status*.json files found — run ./pok capture to generate")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			// Strict unmarshal: fails if the API added fields we haven't mapped.
			var s statusResponse
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&s); err != nil {
				t.Fatalf(
					"unknown or changed fields in status response: %v\n"+
						"Update statusResponse struct and testdata to match the new schema.",
					err,
				)
			}

			// Sanity checks on required fields.
			if s.Status.Indicator == "" {
				t.Error("status.indicator is empty")
			}
			if s.Page.ID == "" {
				t.Error("page.id is empty")
			}
		})
	}
}

func TestReadStatusCache(t *testing.T) {
	t.Parallel()

	t.Run("valid cache returns status", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "status.json")

		resp := &Response{}
		resp.Status.Indicator = "minor"
		resp.Status.Description = "Partially Degraded Service"
		entry := cacheEntry{
			Data:      resp,
			Timestamp: time.Now().Unix(),
			OK:        true,
		}
		jsonfile.Write(cachePath, entry)

		got, err := readCache(cachePath)
		if err != nil {
			t.Fatalf("readCache() error = %v", err)
		}
		if got.Status.Indicator != "minor" {
			t.Errorf("readCache().Status.Indicator = %q, want %q", got.Status.Indicator, "minor")
		}
	})

	t.Run("expired cache returns error", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "status.json")

		resp := &Response{}
		resp.Status.Indicator = "minor"
		entry := cacheEntry{
			Data:      resp,
			Timestamp: time.Now().Add(-ttlOK - time.Second).Unix(),
			OK:        true,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if err == nil {
			t.Error("readCache() error = nil, want error (expired)")
		}
	})

	t.Run("failed cache within TTL returns cached failure error", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "status.json")

		entry := cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        false,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if !errors.Is(err, errCachedFailure) {
			t.Errorf("readCache() error = %v, want %v", err, errCachedFailure)
		}
	})

	t.Run("expired failure returns cache expired", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "status.json")

		entry := cacheEntry{
			Timestamp: time.Now().Add(-ttlFail - time.Second).Unix(),
			OK:        false,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if err == nil {
			t.Error("readCache() error = nil, want error (expired)")
		}
		if errors.Is(err, errCachedFailure) {
			t.Errorf("readCache() error = %v, want cache expired (not sentinel)", err)
		}
	})

	t.Run("ok cache with nil data returns error", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "status.json")

		entry := cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      nil,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if err == nil {
			t.Error("readCache() error = nil, want error for nil data")
		}
	})
}

func TestFetch(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit indicator none returns nil", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		resp := &Response{}
		resp.Status.Indicator = "none"
		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      resp,
		})

		got, err := Fetch(ctx, cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for operational status", got)
		}
	})

	t.Run("cache hit indicator minor returns response", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		resp := &Response{}
		resp.Status.Indicator = "minor"
		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      resp,
		})

		got, err := Fetch(ctx, cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got == nil || got.Status.Indicator != "minor" {
			t.Errorf("Fetch() = %+v, want indicator=minor", got)
		}
	})

	t.Run("cached failure returns nil nil", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        false,
		})

		got, err := Fetch(ctx, cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for cached failure", got)
		}
	})

	t.Run("cache miss API returns none", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"status":{"indicator":"none","description":"All Systems Operational"}}`)
		}))
		defer srv.Close()

		orig := statusURL
		statusURL = srv.URL
		t.Cleanup(func() { statusURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		got, err := Fetch(ctx, cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for operational status", got)
		}
	})

	t.Run("cache miss API returns major", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"status":{"indicator":"major","description":"Major System Outage"}}`)
		}))
		defer srv.Close()

		orig := statusURL
		statusURL = srv.URL
		t.Cleanup(func() { statusURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		got, err := Fetch(ctx, cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got == nil || got.Status.Indicator != "major" {
			t.Errorf("Fetch() = %+v, want indicator=major", got)
		}
	})

	t.Run("cache miss API failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		orig := statusURL
		statusURL = srv.URL
		t.Cleanup(func() { statusURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "status.json")

		_, err := Fetch(ctx, cachePath)
		if err == nil {
			t.Fatal("Fetch() error = nil, want error")
		}
	})
}
