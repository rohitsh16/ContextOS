package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"contextos/internal/model"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestClientMethods(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer ctx_test_secret" {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": 401, "message": "unauthorized"},
			})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.URL.Path {
		case "/v1/context/plan":
			plan := model.ContextPlan{
				Task:           "Test plan task",
				SelectedTokens: 150,
				EstimatedCost:  0.000045,
			}
			_ = json.NewEncoder(w).Encode(plan)

		case "/v1/context/remember":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"memory_id": "mem_123",
				"status":    "stored",
			})

		case "/v1/context/stats":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"repo_id":      "repo_test",
				"node_count":   42,
				"memory_count": 5,
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	c := New("https://api.contextos.com", "ctx_test_secret")
	c.SetTransport(roundTripFunc(func(req *http.Request) *http.Response {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return &http.Response{
			StatusCode: rec.Code,
			Body:       io.NopCloser(rec.Body),
			Header:     rec.Header(),
		}
	}))

	// Test Plan
	plan, err := c.Plan("Test plan task", "local", 4000)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	if plan.Task != "Test plan task" || plan.SelectedTokens != 150 {
		t.Fatalf("unexpected plan response: %+v", plan)
	}

	// Test Remember
	memID, err := c.Remember("decision", "test content", "user", "repo", 0.9, nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}
	if memID != "mem_123" {
		t.Fatalf("expected mem_123, got %s", memID)
	}

	// Test Stats
	stats, err := c.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats["repo_id"] != "repo_test" {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// Test Unauthorized with bad key
	badClient := New("https://api.contextos.com", "ctx_bad_key")
	badClient.SetTransport(c.httpClient.Transport)
	_, err = badClient.Plan("should fail", "", 4000)
	if err == nil {
		t.Fatal("expected error with bad key")
	}
}
