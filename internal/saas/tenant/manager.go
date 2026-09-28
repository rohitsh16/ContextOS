package tenant

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"contextos/internal/server"
)

var (
	ErrTenantNotFound = errors.New("tenant not found")
)

// Options configures default options for dynamically initialized tenant services.
type Options struct {
	BaseDataDir     string        // Root directory where tenant isolated storage will reside
	DefaultRepoPath string        // Optional fallback repo path if tenant has no specific repo
	StorageType     string        // "file" or "sqlite"
	IdleTTL         time.Duration // Time after which an inactive tenant service is evicted from memory
	DefaultBudget   int           // Default token budget
}

// TenantInstance encapsulates an active service and its access metadata.
type TenantInstance struct {
	TenantID   string
	Service    *server.Service
	DataPath   string
	RepoPath   string
	LastAccess time.Time
}

// Manager coordinates isolated multi-tenant ContextOS instances within a single process.
type Manager struct {
	mu        sync.RWMutex
	options   Options
	instances map[string]*TenantInstance
	repoPaths map[string]string // explicit repo paths per tenant
	closed    bool
}

// NewManager initializes a new multi-tenant manager.
func NewManager(opts Options) (*Manager, error) {
	if opts.BaseDataDir == "" {
		opts.BaseDataDir = filepath.Join(os.TempDir(), "contextos-tenants")
	}
	if opts.StorageType == "" {
		opts.StorageType = "file"
	}
	if opts.IdleTTL <= 0 {
		opts.IdleTTL = 30 * time.Minute
	}
	if opts.DefaultBudget <= 0 {
		opts.DefaultBudget = 4000
	}

	if err := os.MkdirAll(opts.BaseDataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create base data directory: %w", err)
	}

	return &Manager{
		options:   opts,
		instances: make(map[string]*TenantInstance),
		repoPaths: make(map[string]string),
	}, nil
}

// RegisterTenantRepo allows binding an explicit repository path to a tenant.
func (m *Manager) RegisterTenantRepo(tenantID, repoPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.repoPaths[tenantID] = repoPath
}

// GetService retrieves or lazily instantiates the isolated server.Service for a tenant.
func (m *Manager) GetService(tenantID string) (*server.Service, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, errors.New("tenant manager is closed")
	}

	now := time.Now()

	// Check if already active in cache
	if inst, ok := m.instances[tenantID]; ok {
		inst.LastAccess = now
		return inst.Service, nil
	}

	// Prepare isolated tenant paths
	tenantDir := filepath.Join(m.options.BaseDataDir, tenantID)
	dataPath := filepath.Join(tenantDir, "data")
	if err := os.MkdirAll(dataPath, 0700); err != nil {
		return nil, fmt.Errorf("failed to create tenant data dir: %w", err)
	}

	repoPath := m.repoPaths[tenantID]
	if repoPath == "" {
		if m.options.DefaultRepoPath != "" {
			repoPath = m.options.DefaultRepoPath
		} else {
			// Create a tenant-specific repo workspace
			repoPath = filepath.Join(tenantDir, "workspace")
			_ = os.MkdirAll(repoPath, 0700)
		}
	}

	// Instantiate isolated server.Service
	svc, err := server.NewWithOptions(dataPath, repoPath, server.Options{
		StorageType:   m.options.StorageType,
		DefaultBudget: m.options.DefaultBudget,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant service: %w", err)
	}

	// Auto-index initial workspace
	_ = svc.Index()

	inst := &TenantInstance{
		TenantID:   tenantID,
		Service:    svc,
		DataPath:   dataPath,
		RepoPath:   repoPath,
		LastAccess: now,
	}

	m.instances[tenantID] = inst
	return svc, nil
}

// EvictCloses an inactive tenant service from memory.
func (m *Manager) Evict(tenantID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[tenantID]
	if !ok {
		return nil
	}

	delete(m.instances, tenantID)
	inst.Service.Close()
	return nil
}

// EvictIdle removes instances that haven't been accessed for longer than IdleTTL.
func (m *Manager) EvictIdle() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	evicted := 0

	for id, inst := range m.instances {
		if now.Sub(inst.LastAccess) > m.options.IdleTTL {
			inst.Service.Close()
			delete(m.instances, id)
			evicted++
		}
	}

	return evicted
}

// ActiveTenantCount returns the number of in-memory active tenant services.
func (m *Manager) ActiveTenantCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.instances)
}

// Close gracefully closes all active tenant services and releases storage locks.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.closed = true
	for _, inst := range m.instances {
		inst.Service.Close()
	}
	m.instances = make(map[string]*TenantInstance)
	return nil
}
