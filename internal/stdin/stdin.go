package stdin

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"
)

// RateLimit is a single rate limit entry from Claude Code's stdin JSON.
type RateLimit struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"` // Unix timestamp
}

// PromptCache is the main conversation's prompt cache ledger from Claude Code's stdin JSON.
type PromptCache struct {
	Warm          bool   `json:"warm"`
	TTL           string `json:"ttl"`
	ExpiresAt     *int64 `json:"expires_at"` // Unix timestamp
	Requests      int    `json:"requests"`
	Misses        int    `json:"misses"`
	LastMissAt    *int64 `json:"last_miss_at"` // Unix timestamp
	LastMissCause *struct {
		Causes []string `json:"causes"`
	} `json:"last_miss_cause"`
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
			InputTokens              int `json:"input_tokens"`
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
	PromptCache *PromptCache `json:"prompt_cache"`
}

// Parse unmarshals the Claude Code stdin JSON.
func Parse(input []byte) (Data, error) {
	var data Data
	if err := json.Unmarshal(input, &data); err != nil {
		return Data{}, fmt.Errorf("parse stdin JSON: %w", err)
	}
	return data, nil
}

// CacheMiss reports whether the latest request was a prompt cache miss, and logs its causes.
func (d Data) CacheMiss() bool {
	pc, cu := d.PromptCache, d.ContextWindow.CurrentUsage
	if pc == nil || pc.ExpiresAt == nil || pc.LastMissAt == nil || cu == nil {
		return false
	}
	ttl, err := time.ParseDuration(pc.TTL)
	if err != nil {
		log.Printf("prompt_cache: parse ttl: %v", err)
		return false
	}
	// Claude Code rounds expires_at up and last_miss_at down, so they are 0-1s apart when the latest request missed.
	gap := *pc.ExpiresAt - int64(ttl.Seconds()) - *pc.LastMissAt
	if gap < 0 || gap > 1 {
		return false
	}
	// Claude Code's miss threshold, applied to the latest request's tokens alone.
	total := cu.InputTokens + cu.CacheReadInputTokens + cu.CacheCreationInputTokens
	if float64(cu.CacheReadInputTokens) >= 0.95*float64(total) || total-cu.CacheReadInputTokens < 2000 {
		return false
	}
	causes := "undiagnosed"
	if pc.LastMissCause != nil && len(pc.LastMissCause.Causes) > 0 {
		causes = strings.Join(pc.LastMissCause.Causes, ", ")
	}
	log.Printf("prompt_cache: miss %d/%d: %s", pc.Misses, pc.Requests, causes)
	return true
}
