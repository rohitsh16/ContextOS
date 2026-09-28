package ratelimit

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"contextos/internal/saas/auth"
)

// TierConfig defines default rate limits for subscription tiers.
type TierConfig struct {
	QPS   float64
	Burst int
}

var DefaultTierConfigs = map[string]TierConfig{
	auth.TierFree:       {QPS: 5, Burst: 10},
	auth.TierPro:        {QPS: 30, Burst: 50},
	auth.TierTeam:       {QPS: 100, Burst: 150},
	auth.TierEnterprise: {QPS: 500, Burst: 1000},
}

// TokenBucket tracks current token balance for an entity.
type TokenBucket struct {
	mu         sync.Mutex
	capacity   float64
	tokens     float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

// NewTokenBucket creates a token bucket with given capacity and refill rate.
func NewTokenBucket(rate float64, burst int) *TokenBucket {
	return &TokenBucket{
		capacity:   float64(burst),
		tokens:     float64(burst),
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

// Allow attempts to take 1 token. Returns allowed, remaining tokens, and retry-after duration if denied.
func (tb *TokenBucket) Allow() (bool, int, time.Duration) {
	return tb.AllowN(time.Now(), 1)
}

// AllowN attempts to take n tokens at time `now`.
func (tb *TokenBucket) AllowN(now time.Time, n float64) (bool, int, time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	// Refill tokens based on elapsed time
	elapsed := now.Sub(tb.lastRefill).Seconds()
	if elapsed > 0 {
		tb.tokens = math.Min(tb.capacity, tb.tokens+elapsed*tb.refillRate)
		tb.lastRefill = now
	}

	if tb.tokens >= n {
		tb.tokens -= n
		remaining := int(tb.tokens)
		return true, remaining, 0
	}

	// Calculate wait time needed for n tokens
	deficit := n - tb.tokens
	secondsNeeded := deficit / tb.refillRate
	retryAfter := time.Duration(math.Ceil(secondsNeeded * float64(time.Second)))
	if retryAfter < time.Millisecond {
		retryAfter = time.Millisecond
	}

	return false, int(tb.tokens), retryAfter
}

// Limiter manages buckets across tenants and API keys.
type Limiter struct {
	mu      sync.RWMutex
	buckets map[string]*TokenBucket
	configs map[string]TierConfig
}

// NewLimiter creates a multi-tenant rate limiter with default tier limits.
func NewLimiter() *Limiter {
	return &Limiter{
		buckets: make(map[string]*TokenBucket),
		configs: DefaultTierConfigs,
	}
}

// GetBucket returns or creates a bucket for the given identifier and tier.
func (l *Limiter) GetBucket(id, tier string, customQPS int) *TokenBucket {
	l.mu.RLock()
	tb, exists := l.buckets[id]
	l.mu.RUnlock()
	if exists {
		return tb
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Double check
	if tb, exists = l.buckets[id]; exists {
		return tb
	}

	cfg, ok := l.configs[tier]
	if !ok {
		cfg = l.configs[auth.TierFree]
	}

	rate := cfg.QPS
	burst := cfg.Burst
	if customQPS > 0 {
		rate = float64(customQPS)
		burst = customQPS * 2
	}

	tb = NewTokenBucket(rate, burst)
	l.buckets[id] = tb
	return tb
}

// Result of a rate limit check.
type Result struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter time.Duration
}

// Check evaluates whether a request for tenant and key is permitted.
func (l *Limiter) Check(tenant *auth.Tenant, key *auth.APIKey) Result {
	var id string
	var tier string
	var customQPS int

	if key != nil {
		id = "key:" + key.ID
		tier = key.Tier
		customQPS = key.RateLimitQPS
	} else if tenant != nil {
		id = "tenant:" + tenant.ID
		tier = tenant.Tier
	} else {
		id = "anonymous"
		tier = auth.TierFree
	}

	tb := l.GetBucket(id, tier, customQPS)
	allowed, remaining, retryAfter := tb.Allow()

	limit := int(tb.refillRate)

	return Result{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: retryAfter,
	}
}

// Middleware returns an HTTP middleware that enforces rate limiting on authenticated requests.
func (l *Limiter) Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenant := auth.GetTenant(r.Context())
			key := auth.GetAPIKey(r.Context())

			res := l.Check(tenant, key)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(res.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
			resetSec := int(math.Ceil(res.RetryAfter.Seconds()))
			if resetSec <= 0 {
				resetSec = 1
			}
			w.Header().Set("X-RateLimit-Reset", strconv.Itoa(resetSec))

			if !res.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(resetSec))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":        http.StatusTooManyRequests,
						"message":     fmt.Sprintf("rate limit exceeded; limit is %d req/sec", res.Limit),
						"retry_after": resetSec,
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
