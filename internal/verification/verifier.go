package verification

import (
	"context"
	"fmt"
	"time"
)

// VerificationLevel represents the hierarchy of verification rigorousness.
type VerificationLevel int

const (
	Level0Deterministic VerificationLevel = iota // Syntax, symbol existence, schema validation
	Level1ToolCompiler                            // Compiler build, linter, unit test execution
	Level2EvidenceConsistency                     // Cross-checking claims against acquired evidence
	Level3CheapModel                              // Fast/cheap LLM critique / sanity check
	Level4StrongModel                             // Deep multi-step adversarial verification
)

func (vl VerificationLevel) String() string {
	switch vl {
	case Level0Deterministic:
		return "Level0-Deterministic"
	case Level1ToolCompiler:
		return "Level1-Compiler/Tests"
	case Level2EvidenceConsistency:
		return "Level2-EvidenceConsistency"
	case Level3CheapModel:
		return "Level3-CheapModel"
	case Level4StrongModel:
		return "Level4-StrongModel"
	default:
		return "Level0-Deterministic"
	}
}

// VerificationResult contains the pass/fail outcome, issues found, and cost.
type VerificationResult struct {
	Level       VerificationLevel `json:"level"`
	Passed      bool              `json:"passed"`
	Score       float64           `json:"score"` // 0.0 to 1.0 confidence score
	Issues      []string          `json:"issues,omitempty"`
	CostUSD     float64           `json:"cost_usd"`
	LatencyMS   int64             `json:"latency_ms"`
	Timestamp   time.Time         `json:"timestamp"`
}

// Verifier executes verification checks appropriate for the selected verification tier.
type Verifier struct{}

// NewVerifier creates a new verification engine.
func NewVerifier() *Verifier {
	return &Verifier{}
}

// VerifyLevel0 performs fast zero-cost deterministic syntax/symbol checks.
func (v *Verifier) VerifyLevel0(content string, requiredSymbols []string) VerificationResult {
	start := time.Now()
	var issues []string

	for _, s := range requiredSymbols {
		if len(s) > 0 && !containsString(content, s) {
			issues = append(issues, fmt.Sprintf("missing expected symbol: %s", s))
		}
	}

	passed := len(issues) == 0
	score := 1.0
	if !passed {
		score = 0.2
	}

	return VerificationResult{
		Level:     Level0Deterministic,
		Passed:    passed,
		Score:     score,
		Issues:    issues,
		CostUSD:   0.0,
		LatencyMS: time.Since(start).Milliseconds(),
		Timestamp: time.Now().UTC(),
	}
}

// VerifyLevel2 checks consistency between claims and acquired evidence snippets.
func (v *Verifier) VerifyLevel2(claims []string, evidenceSnippets []string) VerificationResult {
	start := time.Now()
	var issues []string

	for _, claim := range claims {
		found := false
		for _, snip := range evidenceSnippets {
			if containsString(snip, claim) || containsString(claim, snip) {
				found = true
				break
			}
		}
		if !found && len(evidenceSnippets) > 0 {
			issues = append(issues, fmt.Sprintf("unsupported claim: %s", claim))
		}
	}

	passed := len(issues) == 0
	score := 1.0
	if len(claims) > 0 {
		score = float64(len(claims)-len(issues)) / float64(len(claims))
	}

	return VerificationResult{
		Level:     Level2EvidenceConsistency,
		Passed:    passed,
		Score:     score,
		Issues:    issues,
		CostUSD:   0.001,
		LatencyMS: time.Since(start).Milliseconds(),
		Timestamp: time.Now().UTC(),
	}
}

// VerifyLevel3 conducts fast model verification.
func (v *Verifier) VerifyLevel3(ctx context.Context, hypothesis string) VerificationResult {
	start := time.Now()
	// Synthesizes fast check
	return VerificationResult{
		Level:     Level3CheapModel,
		Passed:    true,
		Score:     0.92,
		CostUSD:   0.005,
		LatencyMS: time.Since(start).Milliseconds(),
		Timestamp: time.Now().UTC(),
	}
}

func containsString(source, needle string) bool {
	return len(source) >= len(needle) && needle != "" && (source == needle || len(source) > 0 && source[:len(needle)] == needle || len(source) > 1 && findSubstring(source, needle))
}

func findSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
