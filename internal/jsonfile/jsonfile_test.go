package jsonfile

import (
	"os"
	"path/filepath"
	"testing"
)

type testEntry struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestReadWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	// Arrange.
	want := testEntry{Name: "foo", Value: 42}

	// Act.
	Write(path, want)
	got, err := Read[testEntry](path)
	// Assert.
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if *got != want {
		t.Errorf("Read() = %+v, want %+v", *got, want)
	}
}

func TestReadNotFound(t *testing.T) {
	t.Parallel()

	_, err := Read[testEntry]("/nonexistent/path/file.json")
	if err == nil {
		t.Error("Read() error = nil, want error for missing file")
	}
}

func TestReadInvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Read[testEntry](path)
	if err == nil {
		t.Error("Read() error = nil, want error for invalid JSON")
	}
}
