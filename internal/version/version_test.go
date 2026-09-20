package version

import (
	"testing"
)

func TestVersionConstants(t *testing.T) {
	if ApplicationVersion != "0.7.0" {
		t.Errorf("expected ApplicationVersion 0.7.0, got %s", ApplicationVersion)
	}
	if ConfigSchemaVersion != 1 {
		t.Errorf("expected ConfigSchemaVersion 1, got %d", ConfigSchemaVersion)
	}
	if StoreSchemaVersion != 2 {
		t.Errorf("expected StoreSchemaVersion 2, got %d", StoreSchemaVersion)
	}
	if ProtocolVersion != "2026-07-28" {
		t.Errorf("expected ProtocolVersion 2026-07-28, got %s", ProtocolVersion)
	}
	if EngineVersion != "ASC-1.4" {
		t.Errorf("expected EngineVersion ASC-1.4, got %s", EngineVersion)
	}
}

func TestVersionInfo(t *testing.T) {
	info := Info()
	if info == "" {
		t.Error("expected non-empty version info")
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		v1, v2 string
		want   int
	}{
		{"0.7.0", "0.6.0", 1},
		{"0.6.0", "0.7.0", -1},
		{"0.7.0", "0.7.0", 0},
		{"v0.7.0", "0.7.0", 0},
		{"0.7.1", "0.7.0", 1},
		{"1.0.0", "0.9.9", 1},
	}
	for _, tt := range tests {
		got := Compare(tt.v1, tt.v2)
		if got != tt.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", tt.v1, tt.v2, got, tt.want)
		}
	}
}
