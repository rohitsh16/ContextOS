package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrTenantNotFound = errors.New("tenant not found")
	ErrKeyNotFound    = errors.New("api key not found")
	ErrTenantInactive = errors.New("tenant is inactive")
	ErrKeyRevoked     = errors.New("api key has been revoked")
	ErrKeyExpired     = errors.New("api key has expired")
	ErrUnauthorized   = errors.New("invalid or unauthorized api key")
)

// Store defines the persistence contract for tenants and credentials.
type Store interface {
	CreateTenant(tenant *Tenant) error
	GetTenant(id string) (*Tenant, error)
	UpdateTenant(tenant *Tenant) error
	ListTenants() ([]*Tenant, error)

	SaveKey(key *APIKey) error
	GetKey(id string) (*APIKey, error)
	LookupKeyByHash(hash string) (*APIKey, error)
	ListKeysForTenant(tenantID string) ([]*APIKey, error)
	RevokeKey(id string) error

	Authenticate(rawKey string) (*Tenant, *APIKey, error)
}

// MemoryStore is an in-memory, thread-safe Store implementation.
type MemoryStore struct {
	mu      sync.RWMutex
	tenants map[string]*Tenant
	keys    map[string]*APIKey // by key ID
	byHash  map[string]*APIKey // by key SHA-256 hash
}

// NewMemoryStore initializes a new in-memory auth store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tenants: make(map[string]*Tenant),
		keys:    make(map[string]*APIKey),
		byHash:  make(map[string]*APIKey),
	}
}

func (s *MemoryStore) CreateTenant(t *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if t.ID == "" {
		return errors.New("tenant ID cannot be empty")
	}
	if t.CreatedAt.IsZero() {
		t.CreatedAt = time.Now().UTC()
	}
	tCopy := *t
	s.tenants[t.ID] = &tCopy
	return nil
}

func (s *MemoryStore) GetTenant(id string) (*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tenants[id]
	if !ok {
		return nil, ErrTenantNotFound
	}
	tCopy := *t
	return &tCopy, nil
}

func (s *MemoryStore) UpdateTenant(t *Tenant) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tenants[t.ID]; !ok {
		return ErrTenantNotFound
	}
	tCopy := *t
	s.tenants[t.ID] = &tCopy
	return nil
}

func (s *MemoryStore) ListTenants() ([]*Tenant, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	res := make([]*Tenant, 0, len(s.tenants))
	for _, t := range s.tenants {
		tCopy := *t
		res = append(res, &tCopy)
	}
	return res, nil
}

func (s *MemoryStore) SaveKey(k *APIKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if k.ID == "" || k.KeyHash == "" {
		return errors.New("invalid key or key hash")
	}
	kCopy := *k
	s.keys[k.ID] = &kCopy
	s.byHash[k.KeyHash] = &kCopy
	return nil
}

func (s *MemoryStore) GetKey(id string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.keys[id]
	if !ok {
		return nil, ErrKeyNotFound
	}
	kCopy := *k
	return &kCopy, nil
}

func (s *MemoryStore) LookupKeyByHash(hash string) (*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	k, ok := s.byHash[hash]
	if !ok {
		return nil, ErrKeyNotFound
	}
	kCopy := *k
	return &kCopy, nil
}

func (s *MemoryStore) ListKeysForTenant(tenantID string) ([]*APIKey, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var res []*APIKey
	for _, k := range s.keys {
		if k.TenantID == tenantID {
			kCopy := *k
			res = append(res, &kCopy)
		}
	}
	return res, nil
}

func (s *MemoryStore) RevokeKey(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k, ok := s.keys[id]
	if !ok {
		return ErrKeyNotFound
	}
	now := time.Now().UTC()
	k.Revoked = true
	k.RevokedAt = &now
	if hKey, exists := s.byHash[k.KeyHash]; exists {
		hKey.Revoked = true
		hKey.RevokedAt = &now
	}
	return nil
}

func (s *MemoryStore) Authenticate(rawKey string) (*Tenant, *APIKey, error) {
	hash := HashKey(rawKey)
	s.mu.RLock()
	k, ok := s.byHash[hash]
	if !ok {
		s.mu.RUnlock()
		return nil, nil, ErrUnauthorized
	}
	if k.Revoked {
		s.mu.RUnlock()
		return nil, nil, ErrKeyRevoked
	}
	if k.ExpiresAt != nil && time.Now().UTC().After(*k.ExpiresAt) {
		s.mu.RUnlock()
		return nil, nil, ErrKeyExpired
	}

	t, ok := s.tenants[k.TenantID]
	if !ok {
		s.mu.RUnlock()
		return nil, nil, ErrTenantNotFound
	}
	if !t.Active {
		s.mu.RUnlock()
		return nil, nil, ErrTenantInactive
	}

	tCopy := *t
	kCopy := *k
	s.mu.RUnlock()
	return &tCopy, &kCopy, nil
}

// FileStore wraps MemoryStore and persists data to a JSON file.
type FileStore struct {
	*MemoryStore
	filePath string
	saveMu   sync.Mutex
}

type fileStoreData struct {
	Tenants []*Tenant `json:"tenants"`
	Keys    []*APIKey `json:"keys"`
}

// NewFileStore initializes or loads an auth store from a file.
func NewFileStore(filePath string) (*FileStore, error) {
	mem := NewMemoryStore()
	fs := &FileStore{
		MemoryStore: mem,
		filePath:    filePath,
	}

	if err := fs.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load auth store from %s: %w", filePath, err)
	}

	return fs, nil
}

func (f *FileStore) load() error {
	data, err := os.ReadFile(f.filePath)
	if err != nil {
		return err
	}
	var state fileStoreData
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	for _, t := range state.Tenants {
		_ = f.MemoryStore.CreateTenant(t)
	}
	for _, k := range state.Keys {
		_ = f.MemoryStore.SaveKey(k)
	}
	return nil
}

func (f *FileStore) persist() error {
	f.saveMu.Lock()
	defer f.saveMu.Unlock()

	tenants, _ := f.MemoryStore.ListTenants()
	f.MemoryStore.mu.RLock()
	keys := make([]*APIKey, 0, len(f.MemoryStore.keys))
	for _, k := range f.MemoryStore.keys {
		kCopy := *k
		keys = append(keys, &kCopy)
	}
	f.MemoryStore.mu.RUnlock()

	state := fileStoreData{
		Tenants: tenants,
		Keys:    keys,
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(f.filePath), 0700); err != nil {
		return err
	}
	tmp := f.filePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, f.filePath)
}

func (f *FileStore) CreateTenant(t *Tenant) error {
	if err := f.MemoryStore.CreateTenant(t); err != nil {
		return err
	}
	return f.persist()
}

func (f *FileStore) UpdateTenant(t *Tenant) error {
	if err := f.MemoryStore.UpdateTenant(t); err != nil {
		return err
	}
	return f.persist()
}

func (f *FileStore) SaveKey(k *APIKey) error {
	if err := f.MemoryStore.SaveKey(k); err != nil {
		return err
	}
	return f.persist()
}

func (f *FileStore) RevokeKey(id string) error {
	if err := f.MemoryStore.RevokeKey(id); err != nil {
		return err
	}
	return f.persist()
}
