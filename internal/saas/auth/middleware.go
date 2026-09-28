package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

type contextKey string

const (
	tenantContextKey contextKey = "saas.tenant"
	apiKeyContextKey contextKey = "saas.apikey"
)

// TenantContext holds the active tenant and credential for an HTTP request.
type TenantContext struct {
	Tenant *Tenant
	Key    *APIKey
}

// WithTenantContext attaches Tenant and APIKey to the context.
func WithTenantContext(ctx context.Context, tenant *Tenant, key *APIKey) context.Context {
	ctx = context.WithValue(ctx, tenantContextKey, tenant)
	ctx = context.WithValue(ctx, apiKeyContextKey, key)
	return ctx
}

// GetTenant retrieves the authenticated Tenant from the context.
func GetTenant(ctx context.Context) *Tenant {
	if t, ok := ctx.Value(tenantContextKey).(*Tenant); ok {
		return t
	}
	return nil
}

// GetAPIKey retrieves the authenticated APIKey from the context.
func GetAPIKey(ctx context.Context) *APIKey {
	if k, ok := ctx.Value(apiKeyContextKey).(*APIKey); ok {
		return k
	}
	return nil
}

// Middleware creates an HTTP middleware that enforces API key authentication.
func Middleware(store Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rawKey := ExtractRawKey(r)
			if rawKey == "" {
				writeJSONError(w, http.StatusUnauthorized, "missing authentication credential (use Bearer token or X-API-Key header)")
				return
			}

			tenant, key, err := store.Authenticate(rawKey)
			if err != nil {
				switch err {
				case ErrTenantInactive:
					writeJSONError(w, http.StatusForbidden, "tenant account is inactive")
				case ErrKeyRevoked:
					writeJSONError(w, http.StatusUnauthorized, "api key has been revoked")
				case ErrKeyExpired:
					writeJSONError(w, http.StatusUnauthorized, "api key has expired")
				default:
					writeJSONError(w, http.StatusUnauthorized, "invalid api key")
				}
				return
			}

			ctx := WithTenantContext(r.Context(), tenant, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireScope returns a middleware that verifies the authenticated key possesses the required scope.
func RequireScope(requiredScope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := GetAPIKey(r.Context())
			if key == nil || !key.HasScope(requiredScope) {
				writeJSONError(w, http.StatusForbidden, "forbidden: missing required scope '"+requiredScope+"'")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ExtractRawKey extracts the API key from Authorization header or X-API-Key header.
func ExtractRawKey(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	if apiKey := r.Header.Get("X-API-Key"); apiKey != "" {
		return strings.TrimSpace(apiKey)
	}
	return ""
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    status,
			"message": message,
		},
	})
}
