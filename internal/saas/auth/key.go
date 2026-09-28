package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Supported SaaS Tiers
const (
	TierFree       = "free"
	TierPro        = "pro"
	TierTeam       = "team"
	TierEnterprise = "enterprise"
)

// Default permissions / scopes
const (
	ScopeContextRead  = "context:read"
	ScopeContextWrite = "context:write"
	ScopeContextAdmin = "context:admin"
)

// Tenant represents an isolated customer organization or user.
type Tenant struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Tier               string            `json:"tier"`
	Active             bool              `json:"active"`
	MonthlyTokenBudget int64             `json:"monthly_token_budget"`
	MonthlyQueryBudget int64             `json:"monthly_query_budget"`
	WebhookURL         string            `json:"webhook_url,omitempty"`
	WebhookSecret      string            `json:"webhook_secret,omitempty"`
	CreatedAt          time.Time         `json:"created_at"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// APIKey represents an authenticated credential mapped to a Tenant.
type APIKey struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	Name         string     `json:"name"`
	KeyHash      string     `json:"key_hash"`
	KeyPrefix    string     `json:"key_prefix"`
	Tier         string     `json:"tier"`
	Scopes       []string   `json:"scopes"`
	RateLimitQPS int        `json:"rate_limit_qps"`
	CreatedAt    time.Time  `json:"created_at"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Revoked      bool       `json:"revoked"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

// KeyGenerationResult wraps the stored APIKey metadata and the plaintext raw key.
type KeyGenerationResult struct {
	Key    *APIKey `json:"key"`
	RawKey string  `json:"raw_key"`
}

// HashKey computes the SHA-256 hex digest of a raw API key.
func HashKey(rawKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(rawKey)))
	return hex.EncodeToString(sum[:])
}

// GenerateAPIKey creates a new cryptographic API key for a tenant.
func GenerateAPIKey(tenantID, name, tier string, scopes []string, rateLimitQPS int, expiresAt *time.Time) (*KeyGenerationResult, error) {
	if strings.TrimSpace(tenantID) == "" {
		return nil, errors.New("tenant_id is required")
	}
	if tier == "" {
		tier = TierFree
	}
	if len(scopes) == 0 {
		scopes = []string{ScopeContextRead, ScopeContextWrite}
	}

	rawBytes := make([]byte, 24)
	if _, err := rand.Read(rawBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}

	prefix := "ctx_live_"
	tokenPart := hex.EncodeToString(rawBytes)
	rawKey := prefix + tokenPart

	idBytes := make([]byte, 8)
	_, _ = rand.Read(idBytes)
	keyID := "key_" + hex.EncodeToString(idBytes)

	displayPrefix := rawKey[:13] + "..."

	key := &APIKey{
		ID:           keyID,
		TenantID:     tenantID,
		Name:         name,
		KeyHash:      HashKey(rawKey),
		KeyPrefix:    displayPrefix,
		Tier:         tier,
		Scopes:       scopes,
		RateLimitQPS: rateLimitQPS,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    expiresAt,
		Revoked:      false,
	}

	return &KeyGenerationResult{
		Key:    key,
		RawKey: rawKey,
	}, nil
}

// Validate checks if the raw key matches this APIKey and is currently active.
func (k *APIKey) Validate(rawKey string) bool {
	if k == nil || k.Revoked {
		return false
	}
	if k.ExpiresAt != nil && time.Now().UTC().After(*k.ExpiresAt) {
		return false
	}
	expectedHash := HashKey(rawKey)
	return subtle.ConstantTimeCompare([]byte(k.KeyHash), []byte(expectedHash)) == 1
}

// HasScope checks if the APIKey contains the required scope.
func (k *APIKey) HasScope(required string) bool {
	if k == nil || k.Revoked {
		return false
	}
	for _, s := range k.Scopes {
		if s == ScopeContextAdmin || s == required {
			return true
		}
	}
	return false
}
