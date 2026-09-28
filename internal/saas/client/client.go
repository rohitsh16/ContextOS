package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"contextos/internal/model"
	"contextos/internal/saas/metering"
)

// Client interacts with a remote ContextOS SaaS Gateway over HTTP.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New creates a new ContextOS SaaS client.
func New(baseURL, apiKey string) *Client {
	baseURL = strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetTransport allows injecting a custom RoundTripper (e.g. for testing).
func (c *Client) SetTransport(rt http.RoundTripper) {
	c.httpClient.Transport = rt
}

func (c *Client) do(method, path string, bodyIn any, respOut any) error {
	var bodyReader io.Reader
	if bodyIn != nil {
		b, err := json.Marshal(bodyIn)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+path, bodyReader)
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response failed: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errObj struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(respBytes, &errObj); jsonErr == nil && errObj.Error.Message != "" {
			return fmt.Errorf("API error (%d): %s", resp.StatusCode, errObj.Error.Message)
		}
		return fmt.Errorf("HTTP error %d: %s", resp.StatusCode, string(respBytes))
	}

	if respOut != nil {
		if err := json.Unmarshal(respBytes, respOut); err != nil {
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}
	return nil
}

// Plan executes a remote context optimization query.
func (c *Client) Plan(task, modelName string, budget int) (*model.ContextPlan, error) {
	if task == "" {
		return nil, errors.New("task cannot be empty")
	}
	req := map[string]any{
		"task":   task,
		"model":  modelName,
		"budget": budget,
	}
	var plan model.ContextPlan
	if err := c.do(http.MethodPost, "/v1/context/plan", req, &plan); err != nil {
		return nil, err
	}
	return &plan, nil
}

// Remember stores a durable engineering decision or observation remotely.
func (c *Client) Remember(kind, content, author, scope string, confidence float64, tags []string) (string, error) {
	req := map[string]any{
		"kind":       kind,
		"content":    content,
		"author":     author,
		"scope":      scope,
		"confidence": confidence,
		"tags":       tags,
	}
	var res map[string]any
	if err := c.do(http.MethodPost, "/v1/context/remember", req, &res); err != nil {
		return "", err
	}
	memID, _ := res["memory_id"].(string)
	return memID, nil
}

// Search searches for relevant context candidates or memories remotely.
func (c *Client) Search(query string, limit int) ([]any, error) {
	req := map[string]any{
		"query": query,
		"limit": limit,
	}
	var res struct {
		Results []any `json:"results"`
	}
	if err := c.do(http.MethodPost, "/v1/context/search", req, &res); err != nil {
		return nil, err
	}
	return res.Results, nil
}

// Stats returns the remote tenant's runtime and repository statistics.
func (c *Client) Stats() (map[string]any, error) {
	var stats map[string]any
	if err := c.do(http.MethodGet, "/v1/context/stats", nil, &stats); err != nil {
		return nil, err
	}
	return stats, nil
}

// Usage returns the metered consumption aggregate for the current billing cycle.
func (c *Client) Usage() (*metering.TenantUsageAggregate, error) {
	var agg metering.TenantUsageAggregate
	if err := c.do(http.MethodGet, "/v1/usage", nil, &agg); err != nil {
		return nil, err
	}
	return &agg, nil
}

// Invoice generates or returns the latest invoice statement for the tenant.
func (c *Client) Invoice() (*metering.Invoice, error) {
	var inv metering.Invoice
	if err := c.do(http.MethodGet, "/v1/invoice", nil, &inv); err != nil {
		return nil, err
	}
	return &inv, nil
}
