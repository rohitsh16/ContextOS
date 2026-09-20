package extractor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"

	"contextos/internal/model"
	"contextos/internal/textutil"
)

// ExtractedClaim represents a structured claim with bound evidence before persistence.
type ExtractedClaim struct {
	Claim       string               `json:"claim"`
	Kind        string               `json:"kind"` // "decision", "fact", "constraint", "failure", "state"
	Scope       string               `json:"scope"`
	Locations   []string             `json:"locations,omitempty"`
	Evidence    []model.EvidenceItem `json:"evidence"`
	Authority   string               `json:"authority"`
	Confidence  float64              `json:"confidence"`
	Timestamp   time.Time            `json:"timestamp"`
	IsReversal  bool                 `json:"is_reversal,omitempty"`
	NegatesTerm string               `json:"negates_term,omitempty"`
}

var (
	reDecision = regexp.MustCompile(`(?i)(?:we decided(?: to)?|decision:\s*|chosen(?:\s+to)?|adopt(?:ed)?|standardize on|use)\s+([^.\n]+)`)
	reReversal = regexp.MustCompile(`(?i)(?:revert(?:ed)?|no longer use|abandon(?:ed)?|deprecated|instead of\s+([a-zA-Z0-9_-]+)\s+use\s+([a-zA-Z0-9_-]+)|switch(?:ed)? from\s+([a-zA-Z0-9_-]+)\s+to\s+([a-zA-Z0-9_-]+))`)
	reFailure  = regexp.MustCompile(`(?i)(?:error|fail(?:ed|ure)?|panic|fatal|exception|broken|reproduced bug):\s*([^.\n]+)`)
	reConstraint = regexp.MustCompile(`(?i)(?:must not|strictly require[ds]?|constraint:\s*|never allow|invariant:\s*|require[ds]?(?:\s+that)?)\s*([^.\n]+)`)
	reFilePath = regexp.MustCompile(`(?:[a-zA-Z0-9_.-]+/)+[a-zA-Z0-9_.-]+\.[a-zA-Z0-9]+(?::\d+)?`)
	reCommitHash = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
)

// Pipeline executes structured extraction, evidence binding, confidence calibration,
// and conflict resolution.
type Pipeline struct {
	RepoRevision string
}

func NewPipeline(revision string) *Pipeline {
	return &Pipeline{RepoRevision: revision}
}

// ExtractFromEvent processes an event and extracts structured claims with bound evidence.
func (p *Pipeline) ExtractFromEvent(evt model.HookEvent) []ExtractedClaim {
	text := evt.Prompt
	if text == "" {
		text = evt.Output
	}
	if text == "" && evt.ToolName != "" {
		text = fmt.Sprintf("tool %s execution", evt.ToolName)
	}
	return p.ExtractFromText(text, evt.EventType, evt.SessionID, evt.CWD)
}

// ExtractFromText analyzes raw text, normalizes content, extracts candidate claims,
// binds evidence, and calibrates confidence.
func (p *Pipeline) ExtractFromText(rawText, eventType, sessionID, cwd string) []ExtractedClaim {
	lines := strings.Split(rawText, "\n")
	var claims []ExtractedClaim

	// Extract file locations mentioned in text
	locations := extractLocations(rawText)
	commits := extractCommits(rawText)

	baseEvidence := make([]model.EvidenceItem, 0)
	for _, loc := range locations {
		baseEvidence = append(baseEvidence, model.EvidenceItem{
			Type:    "source",
			Path:    loc,
			Weight:  0.90,
			Snippet: loc,
		})
	}
	for _, c := range commits {
		baseEvidence = append(baseEvidence, model.EvidenceItem{
			Type:   "commit",
			ID:     c,
			Weight: 0.85,
		})
	}
	if sessionID != "" {
		baseEvidence = append(baseEvidence, model.EvidenceItem{
			Type:    "session_event",
			ID:      sessionID,
			Snippet: eventType,
			Weight:  0.70,
		})
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 5 {
			continue
		}

		// Check for Reversals / Contradictions first
		if m := reReversal.FindStringSubmatch(line); len(m) > 0 {
			claimText := line
			negates := ""
			if len(m) >= 3 && m[1] != "" && m[2] != "" {
				negates = m[1]
				claimText = fmt.Sprintf("Replaced %s with %s", m[1], m[2])
			} else if len(m) >= 5 && m[3] != "" && m[4] != "" {
				negates = m[3]
				claimText = fmt.Sprintf("Switched from %s to %s", m[3], m[4])
			}
			claim := ExtractedClaim{
				Claim:       claimText,
				Kind:        "decision",
				Scope:       "repository",
				Locations:   locations,
				Evidence:    baseEvidence,
				Authority:   "user",
				Timestamp:   time.Now().UTC(),
				IsReversal:  true,
				NegatesTerm: negates,
			}
			claim.Confidence = CalibrateConfidence(claim.Authority, claim.Evidence)
			claims = append(claims, claim)
			continue
		}

		// Check for Decisions
		if m := reDecision.FindStringSubmatch(line); len(m) > 1 {
			claim := ExtractedClaim{
				Claim:      strings.TrimSpace(m[0]),
				Kind:       "decision",
				Scope:      "repository",
				Locations:  locations,
				Evidence:   baseEvidence,
				Authority:  "inference",
				Timestamp:  time.Now().UTC(),
			}
			claim.Confidence = CalibrateConfidence(claim.Authority, claim.Evidence)
			claims = append(claims, claim)
			continue
		}

		// Check for Failures / Postmortems
		if m := reFailure.FindStringSubmatch(line); len(m) > 1 {
			claim := ExtractedClaim{
				Claim:      strings.TrimSpace(m[0]),
				Kind:       "failure",
				Scope:      "repository",
				Locations:  locations,
				Evidence:   baseEvidence,
				Authority:  "test",
				Timestamp:  time.Now().UTC(),
			}
			claim.Confidence = CalibrateConfidence(claim.Authority, claim.Evidence)
			claims = append(claims, claim)
			continue
		}

		// Check for Constraints
		if m := reConstraint.FindStringSubmatch(line); len(m) > 1 {
			claim := ExtractedClaim{
				Claim:      strings.TrimSpace(m[0]),
				Kind:       "constraint",
				Scope:      "repository",
				Locations:  locations,
				Evidence:   baseEvidence,
				Authority:  "user",
				Timestamp:  time.Now().UTC(),
			}
			claim.Confidence = CalibrateConfidence(claim.Authority, claim.Evidence)
			claims = append(claims, claim)
			continue
		}
	}

	return claims
}

func extractLocations(s string) []string {
	matches := reFilePath.FindAllString(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

func extractCommits(s string) []string {
	matches := reCommitHash.FindAllString(s, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(matches))
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return out
}

// CalibrateConfidence computes calibrated probabilistic confidence P(valid) in [0.1, 1.0]
// based on authority level, evidence count and evidence weights.
func CalibrateConfidence(authority string, evidence []model.EvidenceItem) float64 {
	authWeight := 0.45
	switch strings.ToLower(authority) {
	case "user", "explicit":
		authWeight = 1.0
	case "test":
		authWeight = 0.95
	case "source", "commit":
		authWeight = 0.90
	case "doc":
		authWeight = 0.80
	case "inference":
		authWeight = 0.50
	}

	evidenceStrength := 0.0
	for _, ev := range evidence {
		w := ev.Weight
		if w <= 0 {
			switch ev.Type {
			case "user":
				w = 0.99
			case "test":
				w = 0.95
			case "source", "commit":
				w = 0.90
			default:
				w = 0.40
			}
		}
		evidenceStrength += w
	}
	// Diminishing returns on evidence accumulation
	evNorm := 1.0 - math.Exp(-evidenceStrength/2.0)

	// Calibrated probability: logistic function over weighted signals
	z := 2.2*authWeight + 1.8*evNorm - 1.2
	prob := 1.0 / (1.0 + math.Exp(-z))

	if prob < 0.1 {
		return 0.1
	}
	if prob > 0.99 {
		return 0.99
	}
	return math.Round(prob*100) / 100
}

// ConflictResolutionResult holds the result of conflict detection and resolution.
type ConflictResolutionResult struct {
	UpdatedExisting []model.Memory
	NewMemory       model.Memory
	IsDuplicate     bool
	Resolved        bool
}

// ResolveConflicts compares candidate memory against existing memories in the store.
// Invariant: historical records are never mutated or silently erased.
// If candidate contradicts m_old, m_old is marked invalidated/superseded with interval [t0, t1],
// and candidate is created with interval [t1, ...].
// If candidate duplicates m_old, evidence is merged and reuse count incremented.
func ResolveConflicts(existing []model.Memory, candidate ExtractedClaim, currentRevision string) ConflictResolutionResult {
	candID := generateMemoryID(candidate.Claim, candidate.Kind)
	candMem := model.Memory{
		ID:                candID,
		Kind:              candidate.Kind,
		Content:           candidate.Claim,
		Claim:             candidate.Claim,
		Scope:             candidate.Scope,
		ValidFromRevision: currentRevision,
		Authority:         candidate.Authority,
		Confidence:        candidate.Confidence,
		TokenCost:         textutil.EstimateTokens(candidate.Claim),
		ReuseCount:        1,
		Locations:         candidate.Locations,
		Evidence:          candidate.Evidence,
		Timestamp:         candidate.Timestamp.Format(time.RFC3339),
	}
	if len(candidate.Locations) > 0 {
		candMem.Location = candidate.Locations[0]
	}

	var updatedExisting []model.Memory

	for i := range existing {
		m := existing[i]
		// Check for duplicate claim in same scope
		if strings.EqualFold(m.Scope, candidate.Scope) &&
			(strings.EqualFold(m.Claim, candidate.Claim) || textutil.Overlap(m.Content, candidate.Claim) > 0.85) {
			// Merge evidence into existing memory
			m.ReuseCount++
			m.LastAccessedAt = time.Now().UTC().Format(time.RFC3339)
			m.Evidence = mergeEvidence(m.Evidence, candidate.Evidence)
			m.Confidence = math.Max(m.Confidence, candidate.Confidence)
			updatedExisting = append(updatedExisting, m)
			return ConflictResolutionResult{
				UpdatedExisting: updatedExisting,
				NewMemory:       m,
				IsDuplicate:     true,
				Resolved:        true,
			}
		}

		// Check for conflict/reversal:
		// If candidate is a reversal or explicitly negates a term present in m.Content/Claim
		isConflict := false
		if candidate.IsReversal && candidate.NegatesTerm != "" {
			if strings.Contains(strings.ToLower(m.Content), strings.ToLower(candidate.NegatesTerm)) {
				isConflict = true
			}
		} else if strings.EqualFold(m.Kind, candidate.Kind) && strings.EqualFold(m.Scope, candidate.Scope) {
			// If both discuss same subsystem but propose opposing directives
			overlap := textutil.Overlap(m.Content, candidate.Claim)
			if overlap > 0.4 && (strings.Contains(candidate.Claim, "revert") || strings.Contains(candidate.Claim, "replace") || strings.Contains(candidate.Claim, "switch")) {
				isConflict = true
			}
		}

		if isConflict {
			// Invalidate existing memory without erasing historical record
			m.InvalidatedAtRevision = currentRevision
			m.SupersededBy = candID
			updatedExisting = append(updatedExisting, m)

			// Point new memory to superseded memory
			candMem.Supersedes = m.ID
		}
	}

	return ConflictResolutionResult{
		UpdatedExisting: updatedExisting,
		NewMemory:       candMem,
		IsDuplicate:     false,
		Resolved:        true,
	}
}

func mergeEvidence(a, b []model.EvidenceItem) []model.EvidenceItem {
	seen := make(map[string]bool)
	out := make([]model.EvidenceItem, 0, len(a)+len(b))
	for _, item := range a {
		key := fmt.Sprintf("%s:%s:%s", item.Type, item.ID, item.Path)
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	for _, item := range b {
		key := fmt.Sprintf("%s:%s:%s", item.Type, item.ID, item.Path)
		if !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

func generateMemoryID(content, kind string) string {
	h := sha256.Sum256([]byte(kind + ":" + strings.TrimSpace(content) + ":" + fmt.Sprintf("%d", time.Now().UnixNano())))
	return hex.EncodeToString(h[:10])
}
