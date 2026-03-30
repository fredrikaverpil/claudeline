package paths

import (
	"path/filepath"
	"testing"
)

func TestConfigDirSuffix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		configDir string
		want      string
	}{
		{
			name:      "empty config dir",
			configDir: "",
			want:      "",
		},
		{
			name:      "custom config dir claude-personal",
			configDir: "/Users/oa/.claude-personal",
			want:      "-81c94270",
		},
		{
			name:      "custom config dir claude-work",
			configDir: "/Users/oa/.claude-work",
			want:      "-1ef5702c",
		},
		{
			name:      "windows config dir claude-personal",
			configDir: `C:\Users\oa\.claude-personal`,
			want:      "-9b705f7c",
		},
		{
			name:      "windows config dir claude-work",
			configDir: `C:\Users\oa\.claude-work`,
			want:      "-34fd078b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ConfigDirSuffix(tt.configDir)
			if got != tt.want {
				t.Errorf("ConfigDirSuffix(%q) = %q, want %q", tt.configDir, got, tt.want)
			}
		})
	}
}

func TestMustCacheFile(t *testing.T) {
	t.Parallel()

	configDir := "/Users/oa/.claude-work"
	suffix := ConfigDirSuffix(configDir)
	cacheDir := CacheDir()

	tests := []struct {
		name     string
		filename string
		want     string
	}{
		{
			name:     "UsageCache",
			filename: "usage.json",
			want:     filepath.Join(cacheDir, "usage"+suffix+".json"),
		},
		{
			name:     "DebugLog",
			filename: "debug.log",
			want:     filepath.Join(cacheDir, "debug"+suffix+".log"),
		},
		{
			name:     "StatusCache",
			filename: "status.json",
			want:     filepath.Join(cacheDir, "status"+suffix+".json"),
		},
		{
			name:     "UpdateCache",
			filename: "update.json",
			want:     filepath.Join(cacheDir, "update"+suffix+".json"),
		},
		{
			name:     "StdinSnapshot",
			filename: "stdin.json",
			want:     filepath.Join(cacheDir, "stdin"+suffix+".json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := MustCacheFile(configDir, tt.filename)
			if got != tt.want {
				t.Errorf("MustCacheFile(%q, %q) = %q, want %q", configDir, tt.filename, got, tt.want)
			}
		})
	}
}

func TestMustCacheFile_NoConfigDir(t *testing.T) {
	t.Parallel()

	cacheDir := CacheDir()
	got := MustCacheFile("", "usage.json")
	want := filepath.Join(cacheDir, "usage.json")
	if got != want {
		t.Errorf("MustCacheFile(%q, %q) = %q, want %q", "", "usage.json", got, want)
	}
}
