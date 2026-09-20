package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"contextos/internal/model"
	"contextos/internal/version"
)

// Tier represents the storage tier of the hierarchical cache.
type Tier string

const (
	TierMemory     Tier = "L1_MEMORY"
	TierRepository Tier = "L2_REPOSITORY"
	TierPersistent Tier = "L3_PERSISTENT"
)

// Key components uniquely identify a context plan.
type Key struct {
	Revision         string `json:"revision"`
	Model            string `json:"model"`
	Budget           int    `json:"budget"`
	AllocatorVersion string `json:"allocator_version"`
	SchemaVersion    int    `json:"schema_version"`
	Task             string `json:"task"`
}

// Hash returns the deterministic SHA256 hex string for the cache key.
func (k Key) Hash() string {
	raw := fmt.Sprintf("%s:%s:%d:%s:%d:%s",
		k.Revision, k.Model, k.Budget, k.AllocatorVersion, k.SchemaVersion, k.Task)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

// Entry wraps a cached ContextPackage with metadata.
type Entry struct {
	Key       Key                  `json:"key"`
	Hash      string               `json:"hash"`
	Package   model.ContextPackage `json:"package"`
	Tier      Tier                 `json:"tier"`
	CreatedAt time.Time            `json:"created_at"`
	ExpiresAt time.Time            `json:"expires_at"`
}

// HierarchicalCache coordinates exact in-memory, repo-local, and persistent L3 disk caching.
type HierarchicalCache struct {
	mu           sync.RWMutex
	memoryCache  map[string]Entry // L1
	repoCacheDir string           // L2
	diskCacheDir string           // L3
	ttl          time.Duration
}

// NewHierarchicalCache initializes a three-tier cache.
func NewHierarchicalCache(repoRoot, globalDir string, ttl time.Duration) *HierarchicalCache {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	repoDir := filepath.Join(repoRoot, ".contextos", "cache")
	if globalDir == "" {
		home, _ := os.UserHomeDir()
		globalDir = filepath.Join(home, ".contextos", "cache")
	}
	_ = os.MkdirAll(repoDir, 0755)
	_ = os.MkdirAll(globalDir, 0755)

	return &HierarchicalCache{
		memoryCache:  make(map[string]Entry),
		repoCacheDir: repoDir,
		diskCacheDir: globalDir,
		ttl:          ttl,
	}
}

// MakeKey creates a canonical Key using current version constants.
func MakeKey(revision, modelName string, budget int, task string) Key {
	return Key{
		Revision:         revision,
		Model:            modelName,
		Budget:           budget,
		AllocatorVersion: version.EngineVersion,
		SchemaVersion:    version.ConfigSchemaVersion,
		Task:             task,
	}
}

// Get queries L1 (memory), falling back to L2 (repo), then L3 (persistent).
func (c *HierarchicalCache) Get(k Key) (model.ContextPackage, Tier, bool) {
	h := k.Hash()

	// 1. Check L1 Memory
	c.mu.RLock()
	if entry, ok := c.memoryCache[h]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			c.mu.RUnlock()
			return entry.Package, TierMemory, true
		}
	}
	c.mu.RUnlock()

	// 2. Check L2 Repo Cache
	if pkg, ok := c.readDiskEntry(filepath.Join(c.repoCacheDir, h+".json"), k); ok {
		c.PutMemory(k, pkg)
		return pkg, TierRepository, true
	}

	// 3. Check L3 Persistent Disk Cache
	if pkg, ok := c.readDiskEntry(filepath.Join(c.diskCacheDir, h+".json"), k); ok {
		c.PutMemory(k, pkg)
		return pkg, TierPersistent, true
	}

	return model.ContextPackage{}, "", false
}

// Put stores the package into L1, L2, and L3.
func (c *HierarchicalCache) Put(k Key, pkg model.ContextPackage) error {
	h := k.Hash()
	now := time.Now()
	entry := Entry{
		Key:       k,
		Hash:      h,
		Package:   pkg,
		Tier:      TierMemory,
		CreatedAt: now,
		ExpiresAt: now.Add(c.ttl),
	}

	c.mu.Lock()
	c.memoryCache[h] = entry
	c.mu.Unlock()

	// Write L2 (repo) and L3 (persistent)
	b, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	_ = os.WriteFile(filepath.Join(c.repoCacheDir, h+".json"), b, 0644)
	_ = os.WriteFile(filepath.Join(c.diskCacheDir, h+".json"), b, 0644)
	return nil
}

// PutMemory puts an entry directly into L1.
func (c *HierarchicalCache) PutMemory(k Key, pkg model.ContextPackage) {
	h := k.Hash()
	now := time.Now()
	c.mu.Lock()
	c.memoryCache[h] = Entry{
		Key:       k,
		Hash:      h,
		Package:   pkg,
		Tier:      TierMemory,
		CreatedAt: now,
		ExpiresAt: now.Add(c.ttl),
	}
	c.mu.Unlock()
}

// InvalidateRevision purges entries matching a specific Git revision.
func (c *HierarchicalCache) InvalidateRevision(rev string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for h, e := range c.memoryCache {
		if e.Key.Revision == rev {
			delete(c.memoryCache, h)
			_ = os.Remove(filepath.Join(c.repoCacheDir, h+".json"))
			_ = os.Remove(filepath.Join(c.diskCacheDir, h+".json"))
			count++
		}
	}
	return count
}

// InvalidateModel purges entries matching a model name.
func (c *HierarchicalCache) InvalidateModel(modelName string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	count := 0
	for h, e := range c.memoryCache {
		if e.Key.Model == modelName {
			delete(c.memoryCache, h)
			_ = os.Remove(filepath.Join(c.repoCacheDir, h+".json"))
			_ = os.Remove(filepath.Join(c.diskCacheDir, h+".json"))
			count++
		}
	}
	return count
}

// InvalidateAll clears all tiers.
func (c *HierarchicalCache) InvalidateAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.memoryCache = make(map[string]Entry)
	_ = os.RemoveAll(c.repoCacheDir)
	_ = os.RemoveAll(c.diskCacheDir)
	_ = os.MkdirAll(c.repoCacheDir, 0755)
	_ = os.MkdirAll(c.diskCacheDir, 0755)
}

func (c *HierarchicalCache) readDiskEntry(path string, expectedKey Key) (model.ContextPackage, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return model.ContextPackage{}, false
	}
	var e Entry
	if err := json.Unmarshal(b, &e); err != nil {
		return model.ContextPackage{}, false
	}
	if time.Now().After(e.ExpiresAt) {
		_ = os.Remove(path)
		return model.ContextPackage{}, false
	}
	// Verify key integrity
	if e.Key.Revision != expectedKey.Revision ||
		e.Key.Model != expectedKey.Model ||
		e.Key.Budget != expectedKey.Budget ||
		e.Key.SchemaVersion != expectedKey.SchemaVersion {
		return model.ContextPackage{}, false
	}
	return e.Package, true
}
