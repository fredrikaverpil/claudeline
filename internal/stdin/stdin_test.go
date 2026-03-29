package stdin

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// rateLimit is a single rate limit entry from Claude Code's stdin JSON.
// ResetsAt is any because Claude Code sends it as a Unix timestamp (number).
type rateLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       any      `json:"resets_at"`
}

// payload is the complete JSON schema received from Claude Code via stdin.
// This struct documents every known field and is used in tests with
// DisallowUnknownFields to detect when Claude Code adds new fields.
// Update this struct and testdata/*.json when the payload changes.
type payload struct {
	SessionID      string  `json:"session_id"`
	SessionName    *string `json:"session_name"`
	TranscriptPath string  `json:"transcript_path"`
	Cwd            string  `json:"cwd"`
	Model          struct {
		ID          string `json:"id"`
		DisplayName string `json:"display_name"`
	} `json:"model"`
	Workspace struct {
		CurrentDir string   `json:"current_dir"`
		ProjectDir string   `json:"project_dir"`
		AddedDirs  []string `json:"added_dirs"`
	} `json:"workspace"`
	Version     string `json:"version"`
	OutputStyle struct {
		Name string `json:"name"`
	} `json:"output_style"`
	Cost struct {
		TotalCostUSD       float64 `json:"total_cost_usd"`
		TotalDurationMs    int64   `json:"total_duration_ms"`
		TotalAPIDurationMs int64   `json:"total_api_duration_ms"`
		TotalLinesAdded    int     `json:"total_lines_added"`
		TotalLinesRemoved  int     `json:"total_lines_removed"`
	} `json:"cost"`
	ContextWindow struct {
		TotalInputTokens  int `json:"total_input_tokens"`
		TotalOutputTokens int `json:"total_output_tokens"`
		ContextWindowSize int `json:"context_window_size"`
		CurrentUsage      *struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
		UsedPercentage      *float64 `json:"used_percentage"`
		RemainingPercentage *float64 `json:"remaining_percentage"`
	} `json:"context_window"`
	Exceeds200kTokens bool `json:"exceeds_200k_tokens"`
	RateLimits        *struct {
		FiveHour *rateLimit `json:"five_hour"`
		SevenDay *rateLimit `json:"seven_day"`
	} `json:"rate_limits"`
}

// TestPayloadDiff compares all testdata files and reports which fields
// differ across them. Run with -v to see the full diff table.
func TestPayloadDiff(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/stdin_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 2 {
		t.Skip("need at least two testdata files to compare")
	}

	// Load all files into flat key→value maps.
	type fileData struct {
		name   string
		fields map[string]any
	}
	payloads := make([]fileData, 0, len(files))
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		payloads = append(payloads, fileData{
			name:   filepath.Base(f),
			fields: flattenJSON("", m),
		})
	}

	// Collect all unique field paths.
	allKeys := map[string]struct{}{}
	for _, p := range payloads {
		for k := range p.fields {
			allKeys[k] = struct{}{}
		}
	}

	// For each field, check if the value differs across any files.
	for key := range allKeys {
		values := make([]string, len(payloads))
		for i, p := range payloads {
			v, ok := p.fields[key]
			switch {
			case !ok:
				values[i] = "<missing>"
			case v == nil:
				values[i] = "<null>"
			default:
				b, err := json.Marshal(v)
				if err != nil {
					t.Fatalf("marshal %s: %v", key, err)
				}
				s := string(b)
				if len(s) > 60 {
					s = s[:57] + "..."
				}
				values[i] = s
			}
		}
		// Check if all values are the same.
		allSame := true
		for _, v := range values[1:] {
			if v != values[0] {
				allSame = false
				break
			}
		}
		if !allSame {
			t.Logf("DIFF %s:", key)
			for i, p := range payloads {
				t.Logf("  %-50s %s", p.name, values[i])
			}
		}
	}
}

// flattenJSON recursively flattens a nested map into dot-separated key paths.
func flattenJSON(prefix string, m map[string]any) map[string]any {
	result := map[string]any{}
	for k, v := range m {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := v.(map[string]any); ok {
			maps.Copy(result, flattenJSON(key, nested))
		} else {
			result[key] = v
		}
	}
	return result
}

func TestParse(t *testing.T) {
	t.Parallel()

	pct := 42.5

	tests := []struct {
		name    string
		input   string
		want    Data
		wantErr bool
	}{
		{
			name:  "valid with all fields",
			input: `{"cwd":"/home/user","model":{"display_name":"Opus"},"context_window":{"used_percentage":42.5}}`,
			want: Data{
				Cwd: "/home/user",
				Model: struct {
					DisplayName string `json:"display_name"`
				}{DisplayName: "Opus"},
				ContextWindow: struct {
					UsedPercentage *float64 `json:"used_percentage"`
				}{UsedPercentage: &pct},
			},
		},
		{
			name:  "used_percentage absent yields nil pointer",
			input: `{"cwd":"/tmp","model":{"display_name":"Sonnet"},"context_window":{}}`,
			want: Data{
				Cwd: "/tmp",
				Model: struct {
					DisplayName string `json:"display_name"`
				}{DisplayName: "Sonnet"},
			},
		},
		{
			name:  "exceeds_200k_tokens true",
			input: `{"cwd":"/tmp","model":{"display_name":"Opus"},"context_window":{},"exceeds_200k_tokens":true}`,
			want: Data{
				Cwd: "/tmp",
				Model: struct {
					DisplayName string `json:"display_name"`
				}{DisplayName: "Opus"},
				Exceeds200kTokens: true,
			},
		},
		{
			name: "rate_limits parsed",
			input: `{"cwd":"/tmp","model":{"display_name":"Opus"},"context_window":{},` +
				`"rate_limits":{"five_hour":{"used_percentage":28.5,"resets_at":1774785600},` +
				`"seven_day":{"used_percentage":2.0,"resets_at":1775383200}}}`,
			want: func() Data {
				pct5h := 28.5
				reset5h := float64(1774785600)
				pct7d := 2.0
				reset7d := float64(1775383200)
				d := Data{
					Cwd: "/tmp",
					Model: struct {
						DisplayName string `json:"display_name"`
					}{DisplayName: "Opus"},
				}
				d.RateLimits = &struct {
					FiveHour *RateLimit `json:"five_hour"`
					SevenDay *RateLimit `json:"seven_day"`
				}{
					FiveHour: &RateLimit{UsedPercentage: &pct5h, ResetsAt: &reset5h},
					SevenDay: &RateLimit{UsedPercentage: &pct7d, ResetsAt: &reset7d},
				}
				return d
			}(),
		},
		{
			name:  "rate_limits null",
			input: `{"cwd":"/tmp","model":{"display_name":"Opus"},"context_window":{},"rate_limits":null}`,
			want: Data{
				Cwd: "/tmp",
				Model: struct {
					DisplayName string `json:"display_name"`
				}{DisplayName: "Opus"},
			},
		},
		{
			name:  "extra unknown fields ignored",
			input: `{"cwd":"/tmp","model":{"display_name":"Opus","id":"claude-opus-4"},"version":"2.0","unknown_field":true}`,
			want: Data{
				Cwd: "/tmp",
				Model: struct {
					DisplayName string `json:"display_name"`
				}{DisplayName: "Opus"},
			},
		},
		{
			name:    "invalid JSON",
			input:   `{not json}`,
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   ``,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := Parse([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Cwd != tt.want.Cwd {
				t.Errorf("Cwd = %q, want %q", got.Cwd, tt.want.Cwd)
			}
			if got.Model.DisplayName != tt.want.Model.DisplayName {
				t.Errorf("Model.DisplayName = %q, want %q", got.Model.DisplayName, tt.want.Model.DisplayName)
			}
			if tt.want.ContextWindow.UsedPercentage == nil {
				if got.ContextWindow.UsedPercentage != nil {
					t.Errorf("UsedPercentage = %v, want nil", *got.ContextWindow.UsedPercentage)
				}
			} else {
				if got.ContextWindow.UsedPercentage == nil {
					t.Fatal("UsedPercentage is nil, want non-nil")
				}
				if *got.ContextWindow.UsedPercentage != *tt.want.ContextWindow.UsedPercentage {
					t.Errorf(
						"UsedPercentage = %v, want %v",
						*got.ContextWindow.UsedPercentage,
						*tt.want.ContextWindow.UsedPercentage,
					)
				}
			}
			if got.Exceeds200kTokens != tt.want.Exceeds200kTokens {
				t.Errorf("Exceeds200kTokens = %v, want %v", got.Exceeds200kTokens, tt.want.Exceeds200kTokens)
			}
			switch {
			case tt.want.RateLimits == nil && got.RateLimits != nil:
				t.Errorf("RateLimits = %+v, want nil", got.RateLimits)
			case tt.want.RateLimits != nil && got.RateLimits == nil:
				t.Error("RateLimits is nil, want non-nil")
			case tt.want.RateLimits != nil && got.RateLimits != nil:
				assertRateLimit(t, "FiveHour", got.RateLimits.FiveHour, tt.want.RateLimits.FiveHour)
				assertRateLimit(t, "SevenDay", got.RateLimits.SevenDay, tt.want.RateLimits.SevenDay)
			}
		})
	}
}

func TestPayloadSchema(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob("testdata/stdin_*.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no testdata/stdin_*.json files found")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}

			// Strict unmarshal: fails if Claude Code added fields we haven't mapped.
			var p payload
			dec := json.NewDecoder(strings.NewReader(string(data)))
			dec.DisallowUnknownFields()
			if err := dec.Decode(&p); err != nil {
				t.Fatalf(
					"unknown or changed fields in stdin payload: %v\nUpdate payload struct and testdata to match the new schema.",
					err,
				)
			}

			// Sanity checks on required fields.
			if p.Cwd == "" {
				t.Error("cwd is empty")
			}
			if p.Model.DisplayName == "" {
				t.Error("model.display_name is empty")
			}
			if p.Version == "" {
				t.Error("version is empty")
			}
			if p.ContextWindow.ContextWindowSize == 0 {
				t.Error("context_window.context_window_size is 0")
			}
		})
	}
}

// assertRateLimit compares two RateLimit pointers for equality.
func assertRateLimit(t *testing.T, name string, got, want *RateLimit) {
	t.Helper()
	switch {
	case want == nil && got != nil:
		t.Errorf("RateLimits.%s = %+v, want nil", name, got)
	case want != nil && got == nil:
		t.Errorf("RateLimits.%s is nil, want non-nil", name)
	case want != nil && got != nil:
		if (got.UsedPercentage == nil) != (want.UsedPercentage == nil) ||
			(got.UsedPercentage != nil && *got.UsedPercentage != *want.UsedPercentage) {
			t.Errorf("RateLimits.%s.UsedPercentage = %v, want %v", name, got.UsedPercentage, want.UsedPercentage)
		}
		if (got.ResetsAt == nil) != (want.ResetsAt == nil) ||
			(got.ResetsAt != nil && *got.ResetsAt != *want.ResetsAt) {
			t.Errorf("RateLimits.%s.ResetsAt = %v, want %v", name, got.ResetsAt, want.ResetsAt)
		}
	}
}
