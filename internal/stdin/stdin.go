package stdin

import (
	"encoding/json"
	"fmt"
)

// RateLimit is a single rate limit entry from Claude Code's stdin JSON.
type RateLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"` // Unix timestamp
}

// Data is the JSON structure received from Claude Code via stdin.
// See Payload in stdin_test.go for the full schema.
type Data struct {
	Cwd   string `json:"cwd"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		ContextWindowSize int      `json:"context_window_size"`
		UsedPercentage    *float64 `json:"used_percentage"`
		CurrentUsage      *struct {
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"current_usage"`
	} `json:"context_window"`
	Effort *struct {
		Level string `json:"level"`
	} `json:"effort"`
	Exceeds200kTokens bool `json:"exceeds_200k_tokens"`
	RateLimits        *struct {
		FiveHour *RateLimit `json:"five_hour"`
		SevenDay *RateLimit `json:"seven_day"`
	} `json:"rate_limits"`
	Cost struct {
		TotalCostUSD float64 `json:"total_cost_usd"`
	} `json:"cost"`
}

// Parse unmarshals the Claude Code stdin JSON.
func Parse(input []byte) (Data, error) {
	var data Data
	if err := json.Unmarshal(input, &data); err != nil {
		return Data{}, fmt.Errorf("parse stdin JSON: %w", err)
	}
	return data, nil
}
