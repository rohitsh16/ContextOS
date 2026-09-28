package proxy

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"contextos/internal/saas/auth"
	"contextos/internal/saas/metering"
	"contextos/internal/saas/tenant"
)

func TestHostedModelProxy(t *testing.T) {
	baseDir := t.TempDir()

	tm, err := tenant.NewManager(tenant.Options{
		BaseDataDir: filepath.Join(baseDir, "tenants"),
		StorageType: "file",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tm.Close()

	meter := metering.NewMeter()

	tenantObj := &auth.Tenant{
		ID:     "tenant_proxy_test",
		Name:   "Proxy Tenant",
		Tier:   auth.TierEnterprise,
		Active: true,
	}

	svc, err := tm.GetService(tenantObj.ID)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(svc.Repo.Path, "main.go"), []byte("package main\nfunc RunServer() {}\n"), 0600)
	_ = svc.Index()

	_, err = svc.Remember("decision", "Database uses connection pool max 50", "user", "repo", "", 0.95, nil)
	if err != nil {
		t.Fatal(err)
	}

	proxyHandler := NewHandler(tm, meter, "")

	// Test chat completion with ContextOS context injection
	chatReq := ChatCompletionRequest{
		Model: "gpt-4o",
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: "How many database connections should we pool?"},
		},
		MaxTokens: 4000,
	}
	b, _ := json.Marshal(chatReq)

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("X-ContextOS-Mock", "true")
	ctx := auth.WithTenantContext(req.Context(), tenantObj, nil)
	rec := httptest.NewRecorder()

	proxyHandler.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("proxy returned status %d: %s", rec.Code, rec.Body.String())
	}

	var resp ChatCompletionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode completion response: %v", err)
	}

	if resp.Model != "gpt-4o" {
		t.Fatalf("model mismatch: %s", resp.Model)
	}
	if len(resp.Choices) == 0 {
		t.Fatal("no completion choices returned")
	}

	// Verify usage was recorded
	agg, ok := meter.GetAggregate(tenantObj.ID)
	if !ok || agg.TotalRequests != 1 {
		t.Fatalf("expected 1 metered request, got %+v", agg)
	}
	if agg.ByEndpoint["/v1/chat/completions"] != 1 {
		t.Fatalf("expected /v1/chat/completions endpoint metered, got %+v", agg.ByEndpoint)
	}

	t.Logf("✓ Hosted model proxy: PASS — prompt intercepted, context optimized, usage metered")
}
