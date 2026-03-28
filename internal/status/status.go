// Package status provides the Atlassian Statuspage API client with file-based caching.
package status

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/jsonfile"
)

// cacheEntry is the on-disk cache format for status data.
type cacheEntry struct {
	Timestamp int64     `json:"timestamp"`
	OK        bool      `json:"ok"`
	Data      *Response `json:"data,omitempty"`
}

var statusURL = "https://status.claude.com/api/v2/status.json"

const (
	ioTimeout = 5 * time.Second

	ttlOK   = 2 * time.Minute
	ttlFail = 30 * time.Second
)

var errCachedFailure = errors.New("cached status failure")

// Response is the API response from the Atlassian Statuspage API.
type Response struct {
	Status struct {
		Indicator   string `json:"indicator"`
		Description string `json:"description"`
	} `json:"status"`
}

// ReadResponse reads a status Response directly from a JSON file.
func ReadResponse(path string) (*Response, error) {
	return jsonfile.Read[Response](path)
}

// Fetch fetches the service status from the Atlassian Statuspage API with caching.
// Returns (nil, nil) when the service is operational or the result is a cached failure.
func Fetch(ctx context.Context, cachePath string) (*Response, error) {
	cached, err := readCache(cachePath)
	if err == nil {
		if cached.Status.Indicator == "none" {
			return nil, nil
		}
		return cached, nil
	}
	if errors.Is(err, errCachedFailure) {
		return nil, nil
	}

	log.Printf("status: fetching")
	status, fetchErr := fetchStatusAPI(ctx)
	if fetchErr != nil {
		writeCache(cachePath, nil, false)
		return nil, fmt.Errorf("fetch status API: %w", fetchErr)
	}

	writeCache(cachePath, status, true)
	if status.Status.Indicator == "none" {
		return nil, nil
	}
	return status, nil
}

// readCache reads and validates the cached status data.
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

// writeCache writes status data to the cache file.
func writeCache(cachePath string, status *Response, ok bool) {
	jsonfile.Write(cachePath, cacheEntry{
		Timestamp: time.Now().Unix(),
		OK:        ok,
		Data:      status,
	})
}

// fetchStatusAPI makes the HTTP request to the Atlassian Statuspage API.
func fetchStatusAPI(ctx context.Context) (*Response, error) {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		log.Printf("status: unexpected status %d", resp.StatusCode)
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	var status Response
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &status, nil
}
