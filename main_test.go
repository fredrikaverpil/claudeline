package main

import (
	"os"
	"testing"

	"github.com/fredrikaverpil/claudeline/internal/creds"
	"github.com/fredrikaverpil/claudeline/internal/paths"
)

func TestKeychainServiceNameWiring(t *testing.T) {
	t.Parallel()

	configDir := "/Users/oa/.claude-work"
	suffix := paths.ConfigDirSuffix(configDir)
	if got := creds.KeychainServiceName(configDir); got != "Claude Code-credentials"+suffix {
		t.Errorf("KeychainServiceName() = %q, want suffix %q", got, suffix)
	}
}

func BenchmarkRun(b *testing.B) {
	// Use testdata files so the benchmark is fully offline.
	stdinFile := "internal/stdin/testdata/stdin_pro_opus.json"
	usageFile := "internal/usage/testdata/usage_pro.json"
	statusFile := "internal/status/testdata/status.json"

	stdinData, err := os.ReadFile(stdinFile)
	if err != nil {
		b.Fatalf("read stdin testdata: %v", err)
	}

	cfg := config{
		usageFile:       usageFile,
		statusFile:      statusFile,
		gitBranchMaxLen: 30,
		cwdMaxLen:       30,
	}

	// Discard stdout to avoid benchmark noise.
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer devNull.Close()
	origStdout := os.Stdout
	os.Stdout = devNull
	defer func() { os.Stdout = origStdout }()

	b.ResetTimer()
	for b.Loop() {
		r, w, err := os.Pipe()
		if err != nil {
			b.Fatal(err)
		}
		if _, err := w.Write(stdinData); err != nil {
			b.Fatal(err)
		}
		w.Close()

		os.Stdin = r
		if err := run(cfg); err != nil {
			b.Fatalf("run: %v", err)
		}
		r.Close()
	}
}
