package stdin

import (
	"encoding/json"
	"fmt"
)

// Data is the JSON structure received from Claude Code via stdin.
// See Payload in stdin_test.go for the full schema.
type Data struct {
	Cwd   string `json:"cwd"`
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	ContextWindow struct {
		UsedPercentage *float64 `json:"used_percentage"`
	} `json:"context_window"`
	Exceeds200kTokens bool `json:"exceeds_200k_tokens"`
}

// Parse unmarshals the Claude Code stdin JSON.
func Parse(input []byte) (Data, error) {
	var data Data
	if err := json.Unmarshal(input, &data); err != nil {
		return Data{}, fmt.Errorf("parse stdin JSON: %w", err)
	}
	return data, nil
}
