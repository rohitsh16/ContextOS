package webhooks

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}

func TestWebhookDispatchAndHMACVerification(t *testing.T) {
	secret := "whsec_supersecretkey123"
	tenantID := "tenant_webhook_test"

	var receivedBody []byte
	var receivedSig string
	var receivedTS string

	d := NewDispatcher()
	d.SetTransport(roundTripFunc(func(r *http.Request) *http.Response {
		receivedSig = r.Header.Get("X-ContextOS-Signature")
		receivedTS = r.Header.Get("X-ContextOS-Timestamp")
		receivedBody, _ = io.ReadAll(r.Body)
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"received":true}`)),
			Header:     make(http.Header),
		}
	}))
	d.RegisterSubscription(Subscription{
		TenantID: tenantID,
		URL:      "https://example.com/webhook",
		Secret:   secret,
		Events:   []string{EventQuotaWarning, EventPlanCompleted},
		Active:   true,
	})

	evt := Event{
		ID:        "evt_001",
		Type:      EventQuotaWarning,
		TenantID:  tenantID,
		Timestamp: time.Now().UTC(),
		Data: map[string]any{
			"usage_percent": 85.5,
			"tokens_used":   85500,
		},
	}

	record, err := d.Dispatch(evt)
	if err != nil {
		t.Fatalf("Dispatch failed: %v", err)
	}
	if record == nil || !record.Success {
		t.Fatalf("webhook delivery failed: %+v", record)
	}

	// Verify HMAC signature
	if !strings.HasPrefix(receivedSig, "sha256=") {
		t.Fatalf("expected sha256= prefix, got %s", receivedSig)
	}
	rawSig := strings.TrimPrefix(receivedSig, "sha256=")

	tsInt, err := strconv.ParseInt(receivedTS, 10, 64)
	if err != nil {
		t.Fatalf("invalid timestamp header: %v", err)
	}

	if !VerifySignature(secret, tsInt, receivedBody, rawSig) {
		t.Fatal("HMAC signature verification failed")
	}

	// Test with wrong secret should fail
	if VerifySignature("wrong_secret", tsInt, receivedBody, rawSig) {
		t.Fatal("HMAC verification should fail with invalid secret")
	}

	// Verify received JSON payload
	var receivedEvt Event
	if err := json.Unmarshal(receivedBody, &receivedEvt); err != nil {
		t.Fatalf("failed to decode received body: %v", err)
	}
	if receivedEvt.ID != evt.ID || receivedEvt.Type != evt.Type {
		t.Fatalf("mismatched payload: %+v", receivedEvt)
	}
}
