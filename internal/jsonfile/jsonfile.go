// Package jsonfile provides generic JSON file read/write.
package jsonfile

import (
	"encoding/json"
	"os"
)

// Read reads and unmarshals a JSON file into T.
func Read[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Write marshals v as JSON and writes it to path.
func Write[T any](path string, v T) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0o600)
}
