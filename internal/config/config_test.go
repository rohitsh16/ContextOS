package config

import (
	"os"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()
	if cfg.Version != 1 {
		t.Errorf("expected Version 1, got %d", cfg.Version)
	}
	if !cfg.Index.Enabled {
		t.Errorf("expected Index.Enabled true")
	}
	if !cfg.Memory.Enabled {
		t.Errorf("expected Memory.Enabled true")
	}
	if !cfg.Retrieval.Lexical || !cfg.Retrieval.Semantic || !cfg.Retrieval.Graph {
		t.Errorf("expected all retrieval modes enabled by default")
	}
	if cfg.Allocator.BudgetMode != "adaptive" {
		t.Errorf("expected budget_mode adaptive, got %s", cfg.Allocator.BudgetMode)
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfg := Default()
	cfg.Project.Name = "custom-test-project"
	cfg.Allocator.BudgetMode = "strict"

	if err := Save(tmpDir, cfg); err != nil {
		t.Fatalf("Save() failed: %v", err)
	}

	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if loaded.Project.Name != "custom-test-project" {
		t.Errorf("expected Project.Name custom-test-project, got %s", loaded.Project.Name)
	}
	if loaded.Allocator.BudgetMode != "strict" {
		t.Errorf("expected Allocator.BudgetMode strict, got %s", loaded.Allocator.BudgetMode)
	}
}

func TestConfigLoadFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "config-fallback-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// In an empty directory, Load should return Default() without error
	loaded, err := Load(tmpDir)
	if err != nil {
		t.Fatalf("Load() on empty dir returned error: %v", err)
	}
	if loaded.Version != 1 {
		t.Errorf("expected default Version 1, got %d", loaded.Version)
	}
}
