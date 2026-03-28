// Package update provides GitHub release version checking with file-based caching.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/jsonfile"
)

// cacheEntry is the on-disk cache format for release data.
type cacheEntry struct {
	Timestamp int64     `json:"timestamp"`
	OK        bool      `json:"ok"`
	Data      *Response `json:"data,omitempty"`
}

var releaseURL = "https://api.github.com/repos/fredrikaverpil/claudeline/releases/latest"

const (
	ioTimeout = 5 * time.Second

	ttlOK   = 24 * time.Hour
	ttlFail = 15 * time.Second
)

var errCachedFailure = errors.New("cached release failure")

// Response is the relevant subset of the GitHub releases API response.
type Response struct {
	TagName string `json:"tag_name"`
}

// ReadResponse reads a release Response directly from a JSON file.
func ReadResponse(path string) (*Response, error) {
	return jsonfile.Read[Response](path)
}

// Fetch checks for a newer release via the GitHub API with caching.
// Returns non-nil only when a newer version exists.
func Fetch(ctx context.Context, currentVersion, cachePath string) (*Response, error) {
	cached, err := readCache(cachePath)
	if err == nil {
		if NewerAvailable(currentVersion, cached.TagName) {
			return cached, nil
		}
		return nil, nil
	}
	if errors.Is(err, errCachedFailure) {
		return nil, nil
	}

	log.Printf("update: fetching")
	release, fetchErr := fetchReleaseAPI(ctx)
	if fetchErr != nil {
		writeCache(cachePath, nil, false)
		return nil, fmt.Errorf("fetch release API: %w", fetchErr)
	}

	writeCache(cachePath, release, true)
	if NewerAvailable(currentVersion, release.TagName) {
		return release, nil
	}
	return nil, nil
}

// NewerAvailable compares two semver strings and returns true if latest > current.
// Strips leading "v" prefix. Returns false if either version cannot be parsed.
func NewerAvailable(current, latest string) bool {
	cur, okC := parseSemver(current)
	lat, okL := parseSemver(latest)
	if !okC || !okL {
		return false
	}
	if lat[0] != cur[0] {
		return lat[0] > cur[0]
	}
	if lat[1] != cur[1] {
		return lat[1] > cur[1]
	}
	return lat[2] > cur[2]
}

// parseSemver parses a version string like "v1.2.3" or "1.2.3" into [major, minor, patch].
func parseSemver(v string) ([3]int, bool) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, false
	}
	var result [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, false
		}
		result[i] = n
	}
	return result, true
}

// readCache reads and validates the cached release data.
func readCache(cachePath string) (*Response, error) {
	entry, err := jsonfile.Read[cacheEntry](cachePath)
	if err != nil {
		return nil, err
	}

	age := time.Since(time.Unix(entry.Timestamp, 0))
	if entry.OK && age < ttlOK {
		if entry.Data == nil {
			return nil, errors.New("cache hit but no data")
		}
		return entry.Data, nil
	}
	if !entry.OK && age < ttlFail {
		return nil, errCachedFailure
	}
	return nil, errors.New("cache expired")
}

// writeCache writes release data to the cache file.
func writeCache(cachePath string, release *Response, ok bool) {
	jsonfile.Write(cachePath, cacheEntry{
		Timestamp: time.Now().Unix(),
		OK:        ok,
		Data:      release,
	})
}

// fetchReleaseAPI makes the HTTP request to the GitHub releases API.
func fetchReleaseAPI(ctx context.Context) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releaseURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "claudeline")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("update: unexpected status %d", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var release Response
	if err := json.Unmarshal(body, &release); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &release, nil
}
