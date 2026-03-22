package usage

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

// usageResponse is the complete JSON schema from the usage API.
// This struct documents every known field and is used in tests with
// DisallowUnknownFields to detect when the API adds new fields.
// Update this struct and testdata/usage_*.json when the schema changes.
type usageResponse struct {
	FiveHour         *quotaLimit `json:"five_hour"`
	SevenDay         *quotaLimit `json:"seven_day"`
	SevenDaySonnet   *quotaLimit `json:"seven_day_sonnet"`
	SevenDayOpus     *quotaLimit `json:"seven_day_opus"`
	SevenDayOAuthApp *quotaLimit `json:"seven_day_oauth_apps"`
	SevenDayCowork   *quotaLimit `json:"seven_day_cowork"`
	IguanaNecktie    *quotaLimit `json:"iguana_necktie"`
	ExtraUsage       *extraUsage `json:"extra_usage"`
}

type quotaLimit struct {
	Utilization float64 `json:"utilization"`
	ResetsAt    *string `json:"resets_at"`
}

type extraUsage struct {
	IsEnabled    bool     `json:"is_enabled"`
	MonthlyLimit *float64 `json:"monthly_limit"`
	UsedCredits  *float64 `json:"used_credits"`
	Utilization  *float64 `json:"utilization"`
}

func TestUsageResponseSchema(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/usage_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Skip("no testdata/usage_*.json files found — run ./pok capture to generate")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			// Strict unmarshal: fails if the API added fields we haven't mapped.
			var u usageResponse
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&u); err != nil {
				t.Fatalf(
					"unknown or changed fields in usage response: %v\n"+
						"Update usageResponse struct and testdata to match the new schema.",
					err,
				)
			}
		})
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{
			name:  "empty returns default",
			value: "",
			want:  ttlRateLimitDefault,
		},
		{
			name:  "integer seconds",
			value: "120",
			want:  120 * time.Second,
		},
		{
			name:  "clamped to max backoff",
			value: "7200",
			want:  ttlRateLimitMaxBackoff,
		},
		{
			name:  "zero returns default",
			value: "0",
			want:  ttlRateLimitDefault,
		},
		{
			name:  "negative returns default",
			value: "-10",
			want:  ttlRateLimitDefault,
		},
		{
			name:  "unparseable returns default",
			value: "not-a-number",
			want:  ttlRateLimitDefault,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseRetryAfter(tt.value)
			if got != tt.want {
				t.Errorf("parseRetryAfter(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestUsageResponseUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Response
	}{
		{
			name: "full response with all fields",
			input: `{
				"five_hour": {"utilization": 8.0, "resets_at": "2026-03-09T11:00:00+00:00"},
				"seven_day": {"utilization": 31.0, "resets_at": "2026-03-15T08:00:00+00:00"},
				"seven_day_sonnet": {"utilization": 12, "resets_at": "2026-03-09T13:00:00+00:00"},
				"seven_day_opus": {"utilization": 45, "resets_at": "2026-03-09T14:00:00+00:00"},
				"seven_day_oauth_apps": null,
				"seven_day_cowork": {"utilization": 5, "resets_at": "2026-03-10T08:00:00+00:00"},
				"iguana_necktie": null,
				"extra_usage": {"is_enabled": true, "monthly_limit": 5000, "used_credits": 1234, "utilization": null}
			}`,
			want: Response{
				FiveHour:       &QuotaLimit{Utilization: 8.0, ResetsAt: "2026-03-09T11:00:00+00:00"},
				SevenDay:       &QuotaLimit{Utilization: 31.0, ResetsAt: "2026-03-15T08:00:00+00:00"},
				SevenDaySonnet: &QuotaLimit{Utilization: 12, ResetsAt: "2026-03-09T13:00:00+00:00"},
				SevenDayOpus:   &QuotaLimit{Utilization: 45, ResetsAt: "2026-03-09T14:00:00+00:00"},
				SevenDayCowork: &QuotaLimit{Utilization: 5, ResetsAt: "2026-03-10T08:00:00+00:00"},
				ExtraUsage: &ExtraUsage{
					IsEnabled:    true,
					MonthlyLimit: new(float64(5000)),
					UsedCredits:  new(float64(1234)),
				},
			},
		},
		{
			name: "minimal response with nulls",
			input: `{
				"five_hour": {"utilization": 0, "resets_at": null},
				"seven_day": {"utilization": 14, "resets_at": "2026-03-13T08:00:00+00:00"},
				"seven_day_sonnet": null,
				"seven_day_opus": null,
				"seven_day_oauth_apps": null,
				"seven_day_cowork": null,
				"iguana_necktie": null,
				"extra_usage": {"is_enabled": false, "monthly_limit": null, "used_credits": null, "utilization": null}
			}`,
			want: Response{
				FiveHour: &QuotaLimit{Utilization: 0},
				SevenDay: &QuotaLimit{Utilization: 14, ResetsAt: "2026-03-13T08:00:00+00:00"},
				ExtraUsage: &ExtraUsage{
					IsEnabled: false,
				},
			},
		},
		{
			name: "enterprise response with null quotas",
			input: `{
				"five_hour": null,
				"seven_day": null,
				"seven_day_sonnet": null,
				"seven_day_opus": null,
				"seven_day_oauth_apps": null,
				"seven_day_cowork": null,
				"iguana_necktie": null,
				"extra_usage": {"is_enabled": true, "monthly_limit": 10000, "used_credits": 248, "utilization": 2.48}
			}`,
			want: Response{
				ExtraUsage: &ExtraUsage{
					IsEnabled:    true,
					MonthlyLimit: new(float64(10000)),
					UsedCredits:  new(float64(248)),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got Response
			if err := json.Unmarshal([]byte(tt.input), &got); err != nil {
				t.Fatalf("Unmarshal() error = %v", err)
			}
			assertQuotaLimitPtr(t, "FiveHour", got.FiveHour, tt.want.FiveHour)
			assertQuotaLimitPtr(t, "SevenDay", got.SevenDay, tt.want.SevenDay)
			assertQuotaLimitPtr(t, "SevenDaySonnet", got.SevenDaySonnet, tt.want.SevenDaySonnet)
			assertQuotaLimitPtr(t, "SevenDayOpus", got.SevenDayOpus, tt.want.SevenDayOpus)
			assertQuotaLimitPtr(t, "SevenDayOAuthApp", got.SevenDayOAuthApp, tt.want.SevenDayOAuthApp)
			assertQuotaLimitPtr(t, "SevenDayCowork", got.SevenDayCowork, tt.want.SevenDayCowork)
			assertExtraUsage(t, got.ExtraUsage, tt.want.ExtraUsage)
		})
	}
}

func assertQuotaLimitPtr(t *testing.T, name string, got, want *QuotaLimit) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if (got == nil) != (want == nil) {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if *got != *want {
		t.Errorf("%s = %+v, want %+v", name, *got, *want)
	}
}

func assertExtraUsage(t *testing.T, got, want *ExtraUsage) {
	t.Helper()
	if got == nil && want == nil {
		return
	}
	if (got == nil) != (want == nil) {
		t.Errorf("ExtraUsage: got %v, want %v", got, want)
		return
	}
	if got.IsEnabled != want.IsEnabled {
		t.Errorf("ExtraUsage.IsEnabled = %v, want %v", got.IsEnabled, want.IsEnabled)
	}
	assertFloat64Ptr(t, "ExtraUsage.MonthlyLimit", got.MonthlyLimit, want.MonthlyLimit)
	assertFloat64Ptr(t, "ExtraUsage.UsedCredits", got.UsedCredits, want.UsedCredits)
}

func assertFloat64Ptr(t *testing.T, name string, got, want *float64) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Errorf("%s: got %v, want %v", name, got, want)
		return
	}
	if got != nil && *got != *want {
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}

func TestReadCacheRateLimited(t *testing.T) {
	t.Parallel()

	t.Run("rate limited with future RetryAfter returns sentinel error", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "usage.json")

		entry := cacheEntry{
			Timestamp:   time.Now().Unix(),
			OK:          false,
			RateLimited: true,
			RetryAfter:  time.Now().Add(5 * time.Minute).Unix(),
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if !errors.Is(err, ErrCachedRateLimited) {
			t.Errorf("readCache() error = %v, want %v", err, ErrCachedRateLimited)
		}
	})

	t.Run("rate limited with past RetryAfter returns cache expired", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "usage.json")

		entry := cacheEntry{
			Timestamp:   time.Now().Add(-time.Minute).Unix(),
			OK:          false,
			RateLimited: true,
			RetryAfter:  time.Now().Add(-time.Second).Unix(),
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if err == nil || errors.Is(err, ErrCachedRateLimited) {
			t.Errorf("readCache() error = %v, want cache expired", err)
		}
	})

	t.Run("rate limited without RetryAfter uses default TTL fallback", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "usage.json")

		// Simulates cache written by an older version without RetryAfter.
		entry := cacheEntry{
			Timestamp:   time.Now().Unix(),
			OK:          false,
			RateLimited: true,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if !errors.Is(err, ErrCachedRateLimited) {
			t.Errorf("readCache() error = %v, want %v", err, ErrCachedRateLimited)
		}
	})

	t.Run("rate limited without RetryAfter expired returns cache expired", func(t *testing.T) {
		t.Parallel()
		cachePath := filepath.Join(t.TempDir(), "usage.json")

		entry := cacheEntry{
			Timestamp:   time.Now().Add(-ttlRateLimitDefault - time.Second).Unix(),
			OK:          false,
			RateLimited: true,
		}
		jsonfile.Write(cachePath, entry)

		_, err := readCache(cachePath)
		if err == nil || errors.Is(err, ErrCachedRateLimited) {
			t.Errorf("readCache() error = %v, want cache expired", err)
		}
	})
}

// TestReadCacheFailure tests that a failed cache entry within TTL returns the failure sentinel.
func TestReadCacheFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "usage.json")

	entry := cacheEntry{
		Timestamp: time.Now().Unix(),
		OK:        false,
	}
	jsonfile.Write(cachePath, entry)

	_, err := readCache(cachePath)
	if !errors.Is(err, ErrCachedFailure) {
		t.Errorf("readCache() error = %v, want %v", err, ErrCachedFailure)
	}
}

// TestReadCacheValid tests that a valid cache entry returns the usage data.
func TestReadCacheValid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "usage.json")

	entry := cacheEntry{
		Data:      &Response{FiveHour: &QuotaLimit{Utilization: 10, ResetsAt: "2026-03-09T11:00:00+00:00"}},
		Timestamp: time.Now().Unix(),
		OK:        true,
	}
	jsonfile.Write(cachePath, entry)

	got, err := readCache(cachePath)
	if err != nil {
		t.Fatalf("readCache() error = %v", err)
	}
	if got.FiveHour == nil || got.FiveHour.Utilization != 10 {
		t.Errorf("readCache() = %+v, want FiveHour.Utilization=10", got)
	}
}

// TestReadCacheMissing tests that a missing cache file returns an error.
func TestReadCacheMissing(t *testing.T) {
	t.Parallel()

	_, err := readCache("/nonexistent/usage.json")
	if err == nil {
		t.Error("readCache() error = nil, want error for missing file")
	}
	if errors.Is(err, ErrCachedRateLimited) || errors.Is(err, ErrCachedFailure) {
		t.Errorf("readCache() error = %v, want generic error (not sentinel)", err)
	}
}

// TestReadCacheExpired tests that an expired OK cache entry returns an error.
func TestReadCacheExpired(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "usage.json")

	entry := cacheEntry{
		Data:      &Response{FiveHour: &QuotaLimit{Utilization: 10}},
		Timestamp: time.Now().Add(-ttlOK - time.Second).Unix(),
		OK:        true,
	}
	jsonfile.Write(cachePath, entry)

	_, err := readCache(cachePath)
	if err == nil {
		t.Error("readCache() error = nil, want error (expired)")
	}
}

// TestReadCacheOKNilData tests that a valid cache entry with nil data returns an error.
func TestReadCacheOKNilData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "usage.json")

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
	if errors.Is(err, ErrCachedRateLimited) || errors.Is(err, ErrCachedFailure) {
		t.Errorf("readCache() error = %v, want generic error (not sentinel)", err)
	}
}

// TestReadCacheExpiredFailure tests that an expired failure cache entry returns "cache expired".
func TestReadCacheExpiredFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cachePath := filepath.Join(dir, "usage.json")

	entry := cacheEntry{
		Timestamp: time.Now().Add(-ttlFail - time.Second).Unix(),
		OK:        false,
	}
	jsonfile.Write(cachePath, entry)

	_, err := readCache(cachePath)
	if err == nil {
		t.Error("readCache() error = nil, want error (expired)")
	}
	if errors.Is(err, ErrCachedFailure) {
		t.Errorf("readCache() error = %v, want cache expired (not sentinel)", err)
	}
}

func TestParseRetryAfter_RFC1123(t *testing.T) {
	t.Parallel()

	t.Run("future date", func(t *testing.T) {
		t.Parallel()

		future := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC1123)
		got := parseRetryAfter(future)
		// Should be roughly 2 minutes (allow 5s tolerance).
		if got < time.Minute || got > 3*time.Minute {
			t.Errorf("parseRetryAfter(%q) = %v, want ~2m", future, got)
		}
	})

	t.Run("past date returns default", func(t *testing.T) {
		t.Parallel()

		past := time.Now().Add(-time.Hour).UTC().Format(time.RFC1123)
		got := parseRetryAfter(past)
		if got != ttlRateLimitDefault {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", past, got, ttlRateLimitDefault)
		}
	})
}

func TestFetch(t *testing.T) {
	ctx := context.Background()

	t.Run("cache hit returns cached data", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		want := &Response{FiveHour: &QuotaLimit{Utilization: 25}}
		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        true,
			Data:      want,
		})

		got, err := Fetch(ctx, "tok", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got.FiveHour == nil || got.FiveHour.Utilization != 25 {
			t.Errorf("Fetch() = %+v, want FiveHour.Utilization=25", got)
		}
	})

	t.Run("cached rate limit returns sentinel", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp:   time.Now().Unix(),
			OK:          false,
			RateLimited: true,
			RetryAfter:  time.Now().Add(5 * time.Minute).Unix(),
		})

		_, err := Fetch(ctx, "tok", cachePath)
		if !errors.Is(err, ErrCachedRateLimited) {
			t.Errorf("Fetch() error = %v, want %v", err, ErrCachedRateLimited)
		}
	})

	t.Run("cached failure returns sentinel", func(t *testing.T) {
		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		jsonfile.Write(cachePath, cacheEntry{
			Timestamp: time.Now().Unix(),
			OK:        false,
		})

		_, err := Fetch(ctx, "tok", cachePath)
		if !errors.Is(err, ErrCachedFailure) {
			t.Errorf("Fetch() error = %v, want %v", err, ErrCachedFailure)
		}
	})

	t.Run("cache miss with API success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			fmt.Fprintf(w, `{"five_hour":{"utilization":10,"resets_at":"2026-03-09T11:00:00+00:00"}}`)
		}))
		defer srv.Close()

		orig := usageURL
		usageURL = srv.URL
		t.Cleanup(func() { usageURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		got, err := Fetch(ctx, "tok", cachePath)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got.FiveHour == nil || got.FiveHour.Utilization != 10 {
			t.Errorf("Fetch() = %+v, want FiveHour.Utilization=10", got)
		}
	})

	t.Run("cache miss with API 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		orig := usageURL
		usageURL = srv.URL
		t.Cleanup(func() { usageURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		_, err := Fetch(ctx, "tok", cachePath)
		if err == nil {
			t.Fatal("Fetch() error = nil, want error")
		}
	})

	t.Run("cache miss with API 429", func(t *testing.T) {
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls++
			w.Header().Set("Retry-After", "300")
			w.WriteHeader(http.StatusTooManyRequests)
		}))
		defer srv.Close()

		orig := usageURL
		usageURL = srv.URL
		t.Cleanup(func() { usageURL = orig })

		dir := t.TempDir()
		cachePath := filepath.Join(dir, "usage.json")

		_, err := Fetch(ctx, "tok", cachePath)
		if err == nil {
			t.Fatal("Fetch() error = nil, want error")
		}
		// Should only make 1 request (Retry-After is "300", not "0").
		if calls != 1 {
			t.Errorf("expected 1 API call, got %d", calls)
		}
	})
}
