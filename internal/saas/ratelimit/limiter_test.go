package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"contextos/internal/saas/auth"
)

func TestTokenBucketRefillAndDepletion(t *testing.T) {
	// 10 req/s, burst 2
	tb := NewTokenBucket(10, 2)

	// First 2 calls should succeed immediately
	allowed, remaining, _ := tb.Allow()
	if !allowed || remaining != 1 {
		t.Fatalf("first token failed: allowed=%v, remaining=%d", allowed, remaining)
	}

	allowed, remaining, _ = tb.Allow()
	if !allowed || remaining != 0 {
		t.Fatalf("second token failed: allowed=%v, remaining=%d", allowed, remaining)
	}

	// Third call should fail (burst exhausted)
	allowed, _, retryAfter := tb.Allow()
	if allowed {
		t.Fatal("third call should be rate limited")
	}
	if retryAfter <= 0 {
		t.Fatalf("expected positive retryAfter, got %v", retryAfter)
	}

	// Fast-forward simulated time
	now := time.Now().Add(200 * time.Millisecond) // 200ms at 10/s = 2 tokens refilled
	allowed, remaining, _ = tb.AllowN(now, 1)
	if !allowed {
		t.Fatal("call after simulated refill should succeed")
	}
}

func TestLimiterMiddleware(t *testing.T) {
	limiter := NewLimiter()

	tenant := &auth.Tenant{
		ID:   "tenant_test",
		Tier: auth.TierFree,
	}
	// Custom key with burst 2 and QPS 2 for deterministic test
	key := &auth.APIKey{
		ID:           "key_test",
		TenantID:     tenant.ID,
		Tier:         auth.TierFree,
		RateLimitQPS: 2,
	}

	// RateLimitQPS = 2, burst = 4
	bucket := limiter.GetBucket("key:"+key.ID, auth.TierFree, 2)
	_ = bucket

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mw := limiter.Middleware()(handler)

	// Consume all 4 burst tokens
	for i := 0; i < 4; i++ {
		req := httptest.NewRequest(http.MethodGet, "/plan", nil)
		ctx := auth.WithTenantContext(req.Context(), tenant, key)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req.WithContext(ctx))

		if rec.Code != http.StatusOK {
			t.Fatalf("request %d expected 200, got %d", i, rec.Code)
		}
		if rec.Header().Get("X-RateLimit-Limit") == "" {
			t.Fatal("missing X-RateLimit-Limit header")
		}
	}

	// 5th request should be 429
	req := httptest.NewRequest(http.MethodGet, "/plan", nil)
	ctx := auth.WithTenantContext(req.Context(), tenant, key)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 Too Many Requests, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header on 429")
	}
}
