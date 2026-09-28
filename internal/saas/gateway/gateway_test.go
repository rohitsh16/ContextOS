package gateway

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"contextos/internal/mcp"
	"contextos/internal/saas/auth"
	"contextos/internal/saas/metering"
	"contextos/internal/saas/ratelimit"
	"contextos/internal/saas/tenant"
	"contextos/internal/saas/webhooks"
)

func setupTestGateway(t *testing.T) (*Gateway, string, *auth.Tenant, *auth.APIKey, string) {
	t.Helper()
	baseDir := t.TempDir()

	authStore := auth.NewMemoryStore()
	tenantMgr, err := tenant.NewManager(tenant.Options{
		BaseDataDir: filepath.Join(baseDir, "tenants"),
		StorageType: "file",
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	adminKey := "ctx_admin_secret_999"
	limiter := ratelimit.NewLimiter()
	meter := metering.NewMeter()
	disp := webhooks.NewDispatcher()

	gw, err := New(Config{
		AdminAPIKey: adminKey,
		AuthStore:   authStore,
		TenantMgr:   tenantMgr,
		RateLimiter: limiter,
		Meter:       meter,
		WebhookDisp: disp,
	})
	if err != nil {
		t.Fatalf("New Gateway failed: %v", err)
	}

	// Create test tenant
	tnt := &auth.Tenant{
		ID:     "tenant_acme",
		Name:   "Acme Corp",
		Tier:   auth.TierPro,
		Active: true,
	}
	_ = authStore.CreateTenant(tnt)

	// Generate key for tenant
	keyGen, _ := auth.GenerateAPIKey(tnt.ID, "default", auth.TierPro, []string{auth.ScopeContextRead, auth.ScopeContextWrite}, 100, nil)
	_ = authStore.SaveKey(keyGen.Key)

	// Create a dummy file in tenant repo workspace so indexing has content
	tenantSvc, err := tenantMgr.GetService(tnt.ID)
	if err != nil {
		t.Fatalf("GetService failed: %v", err)
	}
	repoPath := tenantSvc.Repo.Path
	_ = os.WriteFile(filepath.Join(repoPath, "main.go"), []byte("package main\nfunc StartApp() {}\n"), 0600)
	_ = tenantSvc.Index()

	return gw, adminKey, tnt, keyGen.Key, keyGen.RawKey
}

func TestGatewayHealthEndpoints(t *testing.T) {
	gw, _, _, _, _ := setupTestGateway(t)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 from /healthz, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"status":"ok"`) {
		t.Fatalf("unexpected body from /healthz: %s", rec.Body.String())
	}
}

func TestGatewayAdminKeyGeneration(t *testing.T) {
	gw, adminKey, _, _, _ := setupTestGateway(t)

	// 1. Without admin key -> 401
	body := `{"tenant_id":"tenant_acme","name":"ci_key","tier":"pro"}`
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/keys", strings.NewReader(body))
	rec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 unauthorized without admin key, got %d", rec.Code)
	}

	// 2. With admin key -> 201 Created
	req2 := httptest.NewRequest(http.MethodPost, "/v1/admin/keys", strings.NewReader(body))
	req2.Header.Set("Authorization", "Bearer "+adminKey)
	rec2 := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusCreated {
		t.Fatalf("expected 201 created from /v1/admin/keys, got %d: %s", rec2.Code, rec2.Body.String())
	}

	var res auth.KeyGenerationResult
	if err := json.Unmarshal(rec2.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode key generation response: %v", err)
	}
	if !strings.HasPrefix(res.RawKey, "ctx_live_") {
		t.Fatalf("expected raw key starting with ctx_live_, got %s", res.RawKey)
	}
}

func TestGatewayPlanAndMetering(t *testing.T) {
	gw, _, tnt, _, rawKey := setupTestGateway(t)

	// 1. Store a memory via HTTP REST
	remBody := `{"kind":"decision","content":"We use Stripe Billing for SaaS","confidence":0.95}`
	remReq := httptest.NewRequest(http.MethodPost, "/v1/context/remember", strings.NewReader(remBody))
	remReq.Header.Set("Authorization", "Bearer "+rawKey)
	remRec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(remRec, remReq)

	if remRec.Code != http.StatusOK {
		t.Fatalf("remember failed: %d %s", remRec.Code, remRec.Body.String())
	}

	// 2. Plan context via HTTP REST
	planBody := `{"task":"Stripe Billing configuration","budget":4000}`
	planReq := httptest.NewRequest(http.MethodPost, "/v1/context/plan", strings.NewReader(planBody))
	planReq.Header.Set("Authorization", "Bearer "+rawKey)
	planRec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(planRec, planReq)

	if planRec.Code != http.StatusOK {
		t.Fatalf("plan failed: %d %s", planRec.Code, planRec.Body.String())
	}

	var planOutput map[string]any
	if err := json.Unmarshal(planRec.Body.Bytes(), &planOutput); err != nil {
		t.Fatalf("failed to unmarshal plan response: %v", err)
	}
	if planOutput["task"] != "Stripe Billing configuration" {
		t.Fatalf("task mismatch: %+v", planOutput)
	}

	// 3. Verify Usage was metered
	useReq := httptest.NewRequest(http.MethodGet, "/v1/usage", nil)
	useReq.Header.Set("Authorization", "Bearer "+rawKey)
	useRec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(useRec, useReq)

	if useRec.Code != http.StatusOK {
		t.Fatalf("usage failed: %d", useRec.Code)
	}
	var usage metering.TenantUsageAggregate
	if err := json.Unmarshal(useRec.Body.Bytes(), &usage); err != nil {
		t.Fatalf("failed to parse usage: %v", err)
	}
	if usage.TotalRequests < 1 {
		t.Fatalf("expected at least 1 metered request, got %d", usage.TotalRequests)
	}
	if usage.TenantID != tnt.ID {
		t.Fatalf("tenant id mismatch: %s", usage.TenantID)
	}

	// 4. Verify Invoice generation
	invReq := httptest.NewRequest(http.MethodGet, "/v1/invoice", nil)
	invReq.Header.Set("Authorization", "Bearer "+rawKey)
	invRec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(invRec, invReq)

	if invRec.Code != http.StatusOK {
		t.Fatalf("invoice failed: %d", invRec.Code)
	}
	var inv metering.Invoice
	if err := json.Unmarshal(invRec.Body.Bytes(), &inv); err != nil {
		t.Fatalf("failed to parse invoice: %v", err)
	}
	if inv.BaseFeeUSD != 29.0 {
		t.Fatalf("expected $29 for Pro tier, got %f", inv.BaseFeeUSD)
	}
}

func TestGatewayMCPOverHTTP(t *testing.T) {
	gw, _, _, _, rawKey := setupTestGateway(t)

	// Call tools/list over HTTP JSON-RPC 2.0
	rpcPayload := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}
	b, _ := json.Marshal(rpcPayload)

	req := httptest.NewRequest(http.MethodPost, "/v1/mcp", bytes.NewReader(b))
	req.Header.Set("Authorization", "Bearer "+rawKey)
	rec := httptest.NewRecorder()
	gw.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("MCP HTTP request failed: %d %s", rec.Code, rec.Body.String())
	}

	var resp mcp.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse MCP response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("MCP returned error: %+v", resp.Error)
	}

	resultMap, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	tools, ok := resultMap["tools"].([]any)
	if !ok || len(tools) < 10 {
		t.Fatalf("expected >=10 tools from MCP tools/list, got %d", len(tools))
	}
}
