package stdin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
}

// Parse reads and unmarshals the Claude Code stdin JSON from r.
func Parse(r io.Reader) (Data, error) {
	input, err := io.ReadAll(r)
	if err != nil {
		return Data{}, fmt.Errorf("read stdin: %w", err)
	}

	log.Printf("raw stdin: %s", input)

	var data Data
	if err := json.Unmarshal(input, &data); err != nil {
		return Data{}, fmt.Errorf("parse stdin JSON: %w", err)
	}

	return data, nil
}
