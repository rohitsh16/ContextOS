package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Supported Event Types
const (
	EventQuotaWarning   = "quota.warning"
	EventQuotaExceeded  = "quota.exceeded"
	EventPlanCompleted  = "plan.completed"
	EventKeyRevoked     = "key.revoked"
	EventInvoiceCreated = "invoice.created"
)

// Event is the payload sent to tenant webhook endpoints.
type Event struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	TenantID  string    `json:"tenant_id"`
	Timestamp time.Time `json:"timestamp"`
	Data      any       `json:"data"`
}

// Subscription configures a webhook receiver for a tenant.
type Subscription struct {
	TenantID string   `json:"tenant_id"`
	URL      string   `json:"url"`
	Secret   string   `json:"secret"`
	Events   []string `json:"events"`
	Active   bool     `json:"active"`
}

// DeliveryRecord tracks the outcome of a webhook HTTP call.
type DeliveryRecord struct {
	EventID    string    `json:"event_id"`
	TenantID   string    `json:"tenant_id"`
	StatusCode int       `json:"status_code"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
	DeliveredAt time.Time `json:"delivered_at"`
}

// Dispatcher manages webhook subscriptions and dispatches signed events.
type Dispatcher struct {
	mu            sync.RWMutex
	subscriptions map[string]*Subscription // tenantID -> Subscription
	history       []DeliveryRecord
	client        *http.Client
}

// NewDispatcher initializes a webhook dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		subscriptions: make(map[string]*Subscription),
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

// SetTransport allows injecting a custom RoundTripper (e.g. in tests or proxy environments).
func (d *Dispatcher) SetTransport(rt http.RoundTripper) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.client.Transport = rt
}

// RegisterSubscription stores or updates a tenant webhook subscription.
func (d *Dispatcher) RegisterSubscription(sub Subscription) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.subscriptions[sub.TenantID] = &sub
}

// GetSubscription returns the webhook subscription for a tenant.
func (d *Dispatcher) GetSubscription(tenantID string) (*Subscription, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	sub, ok := d.subscriptions[tenantID]
	if !ok {
		return nil, false
	}
	cp := *sub
	return &cp, true
}

// ComputeSignature calculates HMAC-SHA256 signature of (timestamp + "." + payload).
func ComputeSignature(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	tsStr := strconv.FormatInt(timestamp, 10)
	mac.Write([]byte(tsStr + "."))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks if the received signature matches the computed signature.
func VerifySignature(secret string, timestamp int64, payload []byte, signature string) bool {
	expected := ComputeSignature(secret, timestamp, payload)
	return hmac.Equal([]byte(signature), []byte(expected))
}

// Dispatch sends an event to the tenant's webhook URL with HMAC signature.
func (d *Dispatcher) Dispatch(evt Event) (*DeliveryRecord, error) {
	d.mu.RLock()
	sub, ok := d.subscriptions[evt.TenantID]
	d.mu.RUnlock()

	if !ok || !sub.Active || sub.URL == "" {
		return nil, nil // No subscription or inactive
	}

	// Check if subscribed to this event type
	subscribed := false
	for _, e := range sub.Events {
		if e == "*" || e == evt.Type {
			subscribed = true
			break
		}
	}
	if !subscribed {
		return nil, nil
	}

	if evt.Timestamp.IsZero() {
		evt.Timestamp = time.Now().UTC()
	}

	payload, err := json.Marshal(evt)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal webhook event: %w", err)
	}

	timestamp := evt.Timestamp.Unix()
	sig := ComputeSignature(sub.Secret, timestamp, payload)

	req, err := http.NewRequest(http.MethodPost, sub.URL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-ContextOS-Signature", "sha256="+sig)
	req.Header.Set("X-ContextOS-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("User-Agent", "ContextOS-Webhook-Dispatcher/1.0")

	resp, err := d.client.Do(req)
	record := &DeliveryRecord{
		EventID:     evt.ID,
		TenantID:    evt.TenantID,
		DeliveredAt: time.Now().UTC(),
	}

	if err != nil {
		record.Success = false
		record.Error = err.Error()
	} else {
		defer resp.Body.Close()
		record.StatusCode = resp.StatusCode
		record.Success = (resp.StatusCode >= 200 && resp.StatusCode < 300)
		if !record.Success {
			record.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		}
	}

	d.mu.Lock()
	d.history = append(d.history, *record)
	d.mu.Unlock()

	return record, nil
}
