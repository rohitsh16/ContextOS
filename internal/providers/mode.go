package providers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// ProviderMode denotes whether the provider adapter uses live external APIs or offline simulations.
type ProviderMode string

const (
	// ProviderModeMock operates purely offline with deterministic mock returns.
	ProviderModeMock ProviderMode = "mock"

	// ProviderModeReal connects to live vendor endpoints (OpenAI, Anthropic, Gemini).
	ProviderModeReal ProviderMode = "real"
)

// BenchmarkRunType explicitly labels experimental provenance to ensure simulation
// and vendor-billed metrics are never conflated.
type BenchmarkRunType string

const (
	// RunTypeSynthetic denotes synthetic curves and simulated task environments.
	RunTypeSynthetic BenchmarkRunType = "synthetic"

	// RunTypeMock denotes deterministic mock provider execution without network I/O.
	RunTypeMock BenchmarkRunType = "mock_provider"

	// RunTypeReal denotes verified live provider API execution with actual billed tokens.
	RunTypeReal BenchmarkRunType = "real_provider"
)

// ExecutionMetadata stores audit provenance for benchmark tasks and agent invocations.
type ExecutionMetadata struct {
	ExecutionID    string           `json:"execution_id"`
	RunType        BenchmarkRunType `json:"run_type"`
	ProviderMode   ProviderMode     `json:"provider_mode"`
	PricingVersion string           `json:"pricing_version"`
	Timestamp      time.Time        `json:"timestamp"`
}

// NewExecutionMetadata generates a timestamped execution record with a cryptographic ID.
func NewExecutionMetadata(runType BenchmarkRunType, mode ProviderMode, pricingVersion string) ExecutionMetadata {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return ExecutionMetadata{
		ExecutionID:    fmt.Sprintf("exec_%x", hex.EncodeToString(buf)),
		RunType:        runType,
		ProviderMode:   mode,
		PricingVersion: pricingVersion,
		Timestamp:      time.Now().UTC(),
	}
}

// IsBilled returns true only if the execution provenance represents verified live vendor billing.
func (m ExecutionMetadata) IsBilled() bool {
	return m.RunType == RunTypeReal && m.ProviderMode == ProviderModeReal
}
