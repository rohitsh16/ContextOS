package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIKeyGenerationAndValidation(t *testing.T) {
	tenantID := "tenant_acme"
	res, err := GenerateAPIKey(tenantID, "production key", TierPro, []string{ScopeContextRead, ScopeContextWrite}, 50, nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if res.RawKey == "" || len(res.RawKey) < 20 {
		t.Fatalf("invalid raw key: %s", res.RawKey)
	}
	if res.Key.TenantID != tenantID {
		t.Fatalf("tenant mismatch: got %s, want %s", res.Key.TenantID, tenantID)
	}

	// Validate raw key matches
	if !res.Key.Validate(res.RawKey) {
		t.Fatal("key validation failed with correct raw key")
	}

	// Validate bad key rejected
	if res.Key.Validate("ctx_live_wrongtoken") {
		t.Fatal("key validation should fail with incorrect raw key")
	}

	// Validate scopes
	if !res.Key.HasScope(ScopeContextRead) {
		t.Fatal("key should have context:read scope")
	}
	if !res.Key.HasScope(ScopeContextWrite) {
		t.Fatal("key should have context:write scope")
	}
	if res.Key.HasScope(ScopeContextAdmin) {
		t.Fatal("key should not have context:admin scope")
	}

	// Expiration test
	past := time.Now().UTC().Add(-1 * time.Hour)
	expiredKey, _ := GenerateAPIKey(tenantID, "expired", TierFree, nil, 5, &past)
	if expiredKey.Key.Validate(expiredKey.RawKey) {
		t.Fatal("expired key must not validate")
	}
}

func TestStoreAndMiddleware(t *testing.T) {
	store := NewMemoryStore()

	tenant := &Tenant{
		ID:     "tenant_corp",
		Name:   "Corp Inc",
		Tier:   TierEnterprise,
		Active: true,
	}
	if err := store.CreateTenant(tenant); err != nil {
		t.Fatalf("CreateTenant failed: %v", err)
	}

	keyRes, err := GenerateAPIKey(tenant.ID, "main", TierEnterprise, []string{ScopeContextRead, ScopeContextWrite, ScopeContextAdmin}, 100, nil)
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}
	if err := store.SaveKey(keyRes.Key); err != nil {
		t.Fatalf("SaveKey failed: %v", err)
	}

	// Authenticate via store
	authTenant, authKey, err := store.Authenticate(keyRes.RawKey)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}
	if authTenant.ID != tenant.ID || authKey.ID != keyRes.Key.ID {
		t.Fatalf("authenticated entity mismatch: tenant=%s, key=%s", authTenant.ID, authKey.ID)
	}

	// Build HTTP handler wrapped with middleware
	protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tnt := GetTenant(r.Context())
		if tnt == nil {
			t.Fatal("tenant not in context")
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok:" + tnt.ID))
	})

	handler := Middleware(store)(protected)

	// 1. Missing header -> 401
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing auth, got %d", rec1.Code)
	}

	// 2. Invalid Bearer -> 401
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("Authorization", "Bearer ctx_live_invalid")
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for bad token, got %d", rec2.Code)
	}

	// 3. Valid Bearer -> 200
	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.Header.Set("Authorization", "Bearer "+keyRes.RawKey)
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK || rec3.Body.String() != "ok:tenant_corp" {
		t.Fatalf("expected 200 ok:tenant_corp, got %d %s", rec3.Code, rec3.Body.String())
	}

	// 4. Valid X-API-Key -> 200
	req4 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req4.Header.Set("X-API-Key", keyRes.RawKey)
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusOK {
		t.Fatalf("expected 200 for X-API-Key, got %d", rec4.Code)
	}

	// 5. Revocation -> 401
	if err := store.RevokeKey(keyRes.Key.ID); err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}
	req5 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req5.Header.Set("Authorization", "Bearer "+keyRes.RawKey)
	rec5 := httptest.NewRecorder()
	handler.ServeHTTP(rec5, req5)
	if rec5.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 after revocation, got %d", rec5.Code)
	}
}
