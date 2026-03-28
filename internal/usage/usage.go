// Package usage provides the Anthropic OAuth usage API client with file-based caching.
package usage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/fredrikaverpil/claudeline/internal/jsonfile"
)

var usageURL = "https://api.anthropic.com/api/oauth/usage"

const (
	ioTimeout = 5 * time.Second

	ttlOK                  = 60 * time.Second
	ttlFail                = 15 * time.Second
	ttlRateLimitDefault    = 5 * time.Minute
	ttlRateLimitMaxBackoff = 30 * time.Minute
)

var (
	errRateLimited = errors.New("rate limited")

	// ErrCachedRateLimited is returned when a cached rate limit is still active.
	ErrCachedRateLimited = errors.New("cached rate limit")
	// ErrCachedFailure is returned when a cached failure is still within its TTL.
	ErrCachedFailure = errors.New("cached failure")
)

// QuotaLimit is a single usage quota with utilization percentage and reset time.
type QuotaLimit struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    string  `json:"resets_at"`
}

// ExtraUsage is the pay-as-you-go overage info.
type ExtraUsage struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
}

// Response is the API response from the usage endpoint.
type Response struct {
	FiveHour         *QuotaLimit `json:"five_hour"`
	SevenDay         *QuotaLimit `json:"seven_day"`
	SevenDaySonnet   *QuotaLimit `json:"seven_day_sonnet"`
	SevenDayOpus     *QuotaLimit `json:"seven_day_opus"`
	SevenDayOAuthApp *QuotaLimit `json:"seven_day_oauth_apps"`
	SevenDayCowork   *QuotaLimit `json:"seven_day_cowork"`
	ExtraUsage       *ExtraUsage `json:"extra_usage"`
}

// ReadResponse reads a usage Response directly from a JSON file.
func ReadResponse(path string) (*Response, error) {
	return jsonfile.Read[Response](path)
}

// Fetch fetches usage data from the API with file-based caching.
func Fetch(ctx context.Context, token, cachePath string) (*Response, error) {
	// Check cache.
	cached, err := readCache(cachePath)
	if err == nil {
		return cached, nil
	}
	// Respect cached rate limit and failure TTLs — don't re-fetch
	// during the cooldown window, as that would reset the TTL on
	// each failed attempt and prevent recovery.
	if errors.Is(err, ErrCachedRateLimited) || errors.Is(err, ErrCachedFailure) {
		return nil, err
	}

	// Fetch from API.
	log.Printf("usage: fetching")
	usage, retryAfter, fetchErr := fetchUsageAPI(ctx, token)
	if fetchErr != nil {
		writeCache(cachePath, nil, false, retryAfter)
		return nil, fmt.Errorf("fetch usage API: %w", fetchErr)
	}

	writeCache(cachePath, usage, true, 0)
	return usage, nil
}

// cacheEntry is the on-disk cache format for usage data.
type cacheEntry struct {
	Timestamp   int64     `json:"timestamp"`
	OK          bool      `json:"ok"`
	RateLimited bool      `json:"rate_limited,omitempty"`
	RetryAfter  int64     `json:"retry_after,omitempty"` // Unix timestamp; retry allowed after this time.
	Data        *Response `json:"data,omitempty"`
}

// readCache reads and validates the cached usage data.
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
	if !entry.OK && entry.RateLimited {
		if entry.RetryAfter > 0 && time.Now().Unix() < entry.RetryAfter {
			return nil, ErrCachedRateLimited
		}
		// Fallback for cache entries without RetryAfter (e.g. written by older versions).
		if entry.RetryAfter == 0 && age < ttlRateLimitDefault {
			return nil, ErrCachedRateLimited
		}
		// Deadline passed or fallback TTL expired — allow re-fetch.
		return nil, errors.New("cache expired")
	}
	if !entry.OK && age < ttlFail {
		return nil, ErrCachedFailure
	}

	return nil, errors.New("cache expired")
}

// writeCache writes usage data to the cache file.
func writeCache(cachePath string, usage *Response, ok bool, retryAfter time.Duration) {
	entry := cacheEntry{
		Timestamp:   time.Now().Unix(),
		OK:          ok,
		RateLimited: retryAfter > 0,
		Data:        usage,
	}
	if retryAfter > 0 {
		entry.RetryAfter = time.Now().Add(retryAfter).Unix()
	}
	jsonfile.Write(cachePath, entry)
}

// fetchUsageAPI makes the HTTP request to the usage API.
// On rate limit (429), retryAfter contains the duration from the retry-after header.
//
// NOTE: The undocumented OAuth usage API (/api/oauth/usage) has been observed
// to return "Retry-After: 0" on 429 responses. Per the HTTP spec, 0 means
// "retry now", but blindly doing so would hammer the API. To distinguish a
// genuine "retry now" from a bad/unset header, we perform a single immediate
// retry. If the retry also returns 429, we treat "0" as a bad signal and
// fall back to the conservative default TTL (ttlRateLimitDefault).
func fetchUsageAPI(ctx context.Context, token string) (_ *Response, retryAfter time.Duration, _ error) {
	usage, rawRetryAfter, err := doUsageRequest(ctx, token)
	if err == nil {
		return usage, 0, nil
	}
	if !errors.Is(err, errRateLimited) {
		return nil, 0, err
	}

	// If Retry-After is "0", the API claims we can retry immediately.
	// Try once more — if it fails again, treat it as a bad signal.
	if rawRetryAfter != "0" {
		ra := parseRetryAfter(rawRetryAfter)
		return nil, ra, err
	}
	usage, _, err = doUsageRequest(ctx, token)
	if err == nil {
		return usage, 0, nil
	}
	if !errors.Is(err, errRateLimited) {
		return nil, 0, err
	}
	// Second attempt also failed — "0" was a bad signal, use default TTL.
	return nil, ttlRateLimitDefault, err
}

// doUsageRequest performs a single HTTP request to the usage API.
// On 429, it returns errRateLimited along with the raw Retry-After header value.
func doUsageRequest(ctx context.Context, token string) (_ *Response, rawRetryAfter string, _ error) {
	ctx, cancel := context.WithTimeout(ctx, ioTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, usageURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Anthropic-Beta", "oauth-2025-04-20")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("execute request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests {
		raw := resp.Header.Get("Retry-After")
		log.Printf("usage: rate limited, retry-after=%q", raw)
		return nil, raw, fmt.Errorf("status %d, retry-after=%q: %w", resp.StatusCode, raw, errRateLimited)
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("usage: unexpected status %d", resp.StatusCode)
		return nil, "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read response body: %w", err)
	}

	var usage Response
	if err := json.Unmarshal(body, &usage); err != nil {
		return nil, "", fmt.Errorf("decode response: %w", err)
	}
	return &usage, "", nil
}

// parseRetryAfter parses the Retry-After header value as seconds (integer)
// or as an HTTP-date (RFC1123). Returns ttlRateLimitDefault if the
// header is missing, zero, or unparseable, clamped to ttlRateLimitMaxBackoff.
//
// NOTE: Values <= 0 return the default TTL. The caller (fetchUsageAPI)
// handles the "Retry-After: 0" case separately with a single retry.
func parseRetryAfter(value string) time.Duration {
	if value == "" {
		return ttlRateLimitDefault
	}
	// Try as seconds first (most common for APIs).
	// Requires secs > 0 to avoid treating "0" as "retry immediately".
	if secs, err := strconv.Atoi(value); err == nil && secs > 0 {
		d := time.Duration(secs) * time.Second
		return min(d, ttlRateLimitMaxBackoff)
	}
	// Try as HTTP-date (RFC1123).
	if t, err := time.Parse(time.RFC1123, value); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return ttlRateLimitDefault
		}
		return min(d, ttlRateLimitMaxBackoff)
	}
	return ttlRateLimitDefault
}
