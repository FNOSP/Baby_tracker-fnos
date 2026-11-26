package app

import (
	"path/filepath"
	"testing"
)

func TestLoadConfigReadsLegacyDataDir(t *testing.T) {
	primary := filepath.Join(t.TempDir(), "current")
	legacy := filepath.Join(t.TempDir(), "legacy")
	t.Setenv("BABY_TRACKER_DATA_DIR", primary)
	t.Setenv("BABY_TRACKER_LEGACY_DATA_DIR", legacy)

	cfg := LoadConfig()

	if cfg.DataDir != primary {
		t.Fatalf("data dir = %q, want %q", cfg.DataDir, primary)
	}
	if len(cfg.LegacyDataDirs) != 1 || cfg.LegacyDataDirs[0] != legacy {
		t.Fatalf("legacy data dirs = %v, want [%s]", cfg.LegacyDataDirs, legacy)
	}
}

func TestNormalizeGatewayPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty uses default", input: "", want: defaultGatewayPrefix},
		{name: "root uses default", input: "/", want: defaultGatewayPrefix},
		{name: "adds leading slash", input: "app/baby-tracker", want: "/app/baby-tracker"},
		{name: "removes trailing slash", input: "/app/baby-tracker/", want: "/app/baby-tracker"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeGatewayPrefix(tt.input); got != tt.want {
				t.Fatalf("normalizeGatewayPrefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
