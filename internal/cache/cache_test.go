package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"contextos/internal/model"
)

func TestHierarchicalCacheTiering(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	repoDir := filepath.Join(tmpDir, "repo")
	globalDir := filepath.Join(tmpDir, "global")

	c := NewHierarchicalCache(repoDir, globalDir, 1*time.Hour)
	key := MakeKey("rev-1", "claude-3-5-sonnet", 4000, "Fix memory leak")

	pkg := model.ContextPackage{
		SchemaVersion: "1.0.0",
		Task:          "Fix memory leak",
		Model:         "claude-3-5-sonnet",
		Budget:        4000,
	}

	// Initial get should be miss
	_, _, ok := c.Get(key)
	if ok {
		t.Fatal("expected cache miss on empty cache")
	}

	// Put into cache
	if err := c.Put(key, pkg); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Should hit L1 Memory
	got, tier, ok := c.Get(key)
	if !ok || tier != TierMemory {
		t.Fatalf("expected L1 hit, got ok=%t tier=%s", ok, tier)
	}
	if got.Task != "Fix memory leak" {
		t.Errorf("task mismatch: %s", got.Task)
	}

	// Simulate restart by clearing L1 memory cache
	c.memoryCache = make(map[string]Entry)

	// Should hit L2 Repo Cache
	got2, tier2, ok2 := c.Get(key)
	if !ok2 || tier2 != TierRepository {
		t.Fatalf("expected L2 hit, got ok=%t tier=%s", ok2, tier2)
	}
	if got2.Task != "Fix memory leak" {
		t.Errorf("task mismatch: %s", got2.Task)
	}

	// Clear L1 memory and L2 repo cache
	c.memoryCache = make(map[string]Entry)
	_ = os.RemoveAll(filepath.Join(repoDir, ".contextos", "cache"))

	// Should hit L3 Persistent Cache
	got3, tier3, ok3 := c.Get(key)
	if !ok3 || tier3 != TierPersistent {
		t.Fatalf("expected L3 hit, got ok=%t tier=%s", ok3, tier3)
	}
	if got3.Task != "Fix memory leak" {
		t.Errorf("task mismatch: %s", got3.Task)
	}
}

func TestHierarchicalCacheInvalidation(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cache-inval-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	c := NewHierarchicalCache(tmpDir, tmpDir, 1*time.Hour)
	key1 := MakeKey("rev-1", "model-a", 2000, "task 1")
	key2 := MakeKey("rev-2", "model-a", 2000, "task 2")

	_ = c.Put(key1, model.ContextPackage{Task: "task 1"})
	_ = c.Put(key2, model.ContextPackage{Task: "task 2"})

	// Invalidate rev-1
	count := c.InvalidateRevision("rev-1")
	if count != 1 {
		t.Errorf("expected 1 invalidated entry, got %d", count)
	}

	// key1 should be miss, key2 should still hit
	_, _, ok1 := c.Get(key1)
	if ok1 {
		t.Errorf("key1 should have been invalidated")
	}

	_, _, ok2 := c.Get(key2)
	if !ok2 {
		t.Errorf("key2 should still be present")
	}
}
