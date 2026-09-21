package providers

import "context"

// Provider defines the vendor-neutral interface for LLM interaction and capability negotiation.
type Provider interface {
	// Name returns the provider identifier (e.g. "openai", "anthropic", "gemini").
	Name() string

	// Capabilities returns the model-specific capabilities and constraints.
	Capabilities(ctx context.Context, model string) (ModelCapabilities, error)

	// Generate executes an inference request adhering to the provider-neutral ComputePolicy.
	Generate(ctx context.Context, req ProviderRequest) (ProviderResponse, error)
}
