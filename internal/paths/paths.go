// Package paths provides cache and log file path construction for claudeline.
package paths

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// DefaultConfigDir returns the default Claude Code config directory (~/.claude).
func DefaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Sprintf("no user home dir found: %v", err))
	}
	return filepath.Join(home, ".claude")
}

// ConfigDirSuffix returns a hash-based suffix for the given config directory,
// or an empty string when configDir is empty. This avoids collisions between
// Claude Code profiles.
func ConfigDirSuffix(configDir string) string {
	if configDir == "" || configDir == DefaultConfigDir() {
		return ""
	}
	h := sha256.Sum256([]byte(configDir))
	return fmt.Sprintf("-%x", h[:4])
}

// CacheDir returns the directory for claudeline cache and log files.
// Uses /tmp/claudeline on Unix and os.TempDir()/claudeline on Windows.
func CacheDir() string {
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	return filepath.Join(base, "claudeline")
}

// MustCacheFile constructs a filepath into /tmp/claudeline, incorporating a
// configDir-based suffix to avoid collisions between Claude Code profiles.
func MustCacheFile(configDir, filename string) string {
	name, ext, ok := strings.Cut(filename, ".")
	if !ok {
		panic(fmt.Sprintf("cannot cut filename: %s", filename))
	}
	return filepath.Join(CacheDir(), name+ConfigDirSuffix(configDir)+"."+ext)
}
