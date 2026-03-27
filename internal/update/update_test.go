package update

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/jsonfile"
)

func TestReadResponse(t *testing.T) {
	t.Parallel()

	resp, err := ReadResponse("testdata/release.json")
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	if resp.TagName != "v0.13.0" {
		t.Errorf("ReadResponse().TagName = %q, want %q", resp.TagName, "v0.13.0")
	}
}

func TestNewerAvailable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		{name: "newer patch", current: "v0.12.0", latest: "v0.13.0", want: true},
		{name: "newer minor", current: "v0.12.0", latest: "v0.14.0", want: true},
		{name: "newer major", current: "v0.12.0", latest: "v1.0.0", want: true},
		{name: "same version", current: "v0.13.0", latest: "v0.13.0", want: false},
		{name: "older version", current: "v0.14.0", latest: "v0.13.0", want: false},
		{name: "without v prefix", current: "0.12.0", latest: "0.13.0", want: true},
		{name: "mixed v prefix", current: "v0.12.0", latest: "0.13.0", want: true},
		{name: "empty current", current: "", latest: "v0.13.0", want: false},
		{name: "empty latest", current: "v0.12.0", latest: "", want: false},
		{name: "both empty", current: "", latest: "", want: false},
		{name: "devel current", current: "(devel)", latest: "v0.13.0", want: false},
		{name: "unknown current", current: "(unknown)", latest: "v0.13.0", want: false},
		{name: "invalid current", current: "abc", latest: "v0.13.0", want: false},
		{name: "invalid latest", current: "v0.12.0", latest: "abc", want: false},
		{name: "two-part version", current: "v0.12", latest: "v0.13.0", want: false},
		{name: "version with commit", current: "v0.12.0 (abc123)", latest: "v0.13.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := NewerAvailable(tt.current, tt.latest)
			if got != tt.want {
				t.Errorf("NewerAvailable(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
			}
		})
	}
}

func TestReadUpdateCache(t *testing.T) {
	t.Parallel()

	t.Run("valid cache returns response", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "update.json")

		resp := &Response{TagName: "v0.13.0"}
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
		if got.TagName != "v0.13.0" {
			t.Errorf("readCache().TagName = %q, want %q", got.TagName, "v0.13.0")
		}
	})

	t.Run("expired cache returns error", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "update.json")

		resp := &Response{TagName: "v0.13.0"}
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
		cachePath := filepath.Join(t.TempDir(), "update.json")

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
		cachePath := filepath.Join(t.TempDir(), "update.json")

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
		cachePath := filepath.Join(t.TempDir(), "update.json")

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

	t.Run("cache hit newer version", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      &Response{TagName: "v0.14.0"},
		})

		got, err := Fetch(ctx, "v0.13.0", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got == nil || got.TagName != "v0.14.0" {
			t.Errorf("Fetch() = %+v, want tag_name=v0.14.0", got)
		}
	})

	t.Run("cache hit same version", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      &Response{TagName: "v0.13.0"},
		})

		got, err := Fetch(ctx, "v0.13.0", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for same version", got)
		}
	})

	t.Run("cached failure returns nil nil", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        false,
		})

		got, err := Fetch(ctx, "v0.13.0", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for cached failure", got)
		}
	})

	t.Run("cache miss API returns newer", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Accept") != "application/vnd.github+json" {
				t.Errorf("missing Accept header")
			}
			if r.Header.Get("User-Agent") != "claudeline" {
				t.Errorf("missing User-Agent header")
			}
			fmt.Fprintf(w, `{"tag_name":"v0.14.0"}`)
		}))
		defer srv.Close()

		orig := releaseURL
		releaseURL = srv.URL
		t.Cleanup(func() { releaseURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		got, err := Fetch(ctx, "v0.13.0", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got == nil || got.TagName != "v0.14.0" {
			t.Errorf("Fetch() = %+v, want tag_name=v0.14.0", got)
		}
	})

	t.Run("cache miss API returns same version", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"tag_name":"v0.13.0"}`)
		}))
		defer srv.Close()

		orig := releaseURL
		releaseURL = srv.URL
		t.Cleanup(func() { releaseURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		got, err := Fetch(ctx, "v0.13.0", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got != nil {
			t.Errorf("Fetch() = %+v, want nil for same version", got)
		}
	})

	t.Run("cache miss API failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		orig := releaseURL
		releaseURL = srv.URL
		t.Cleanup(func() { releaseURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "update.json")

		_, err := Fetch(ctx, "v0.13.0", cachePath)
		if err == nil {
			t.Fatal("Fetch() error = nil, want error")
		}
	})
}
