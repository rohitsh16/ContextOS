package retrieval

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Claim represents a testable factual or architectural assertion about code (R18 §29).
type Claim struct {
	ID         string  `json:"id"`
	Text       string  `json:"text"`
	Type       string  `json:"type"` // "factual", "call_chain", "condition", "interface"
	Required   bool    `json:"required"`
	Confidence float64 `json:"confidence"`
	Weight     float64 `json:"weight"`
}

// ClaimEvidence links an atomic claim to supporting or conflicting evidence nodes.
type ClaimEvidence struct {
	ClaimID     string   `json:"claim_id"`
	EvidenceIDs []string `json:"evidence_ids"`
	Status      string   `json:"status"` // "SUPPORTED", "PARTIAL", "CONTRADICTED", "UNKNOWN"
	Score       float64  `json:"score"`
}

// QueryContract defines the explicit evidence requirements for a query (R18 §24).
type QueryContract struct {
	Query            string   `json:"query"`
	TaskType         string   `json:"task_type"` // "trace", "lookup", "debug", "refactor"
	Entities         []string `json:"entities"`
	RequiredClaims   []Claim  `json:"required_claims"`
	RequiredEvidence []string `json:"required_evidence"` // paths or symbol IDs
	NeedsCallGraph   bool     `json:"needs_call_graph"`
	NeedsHistory     bool     `json:"needs_history"`
	NeedsTests       bool     `json:"needs_tests"`
	NeedsConfig      bool     `json:"needs_config"`
}

// DecomposeQuery analyzes a query string to build a deterministic QueryContract (R18 §24).
func DecomposeQuery(query string) QueryContract {
	qLower := strings.ToLower(query)
	contract := QueryContract{
		Query: query,
	}

	// Task Type inference
	if strings.Contains(qLower, "trace") || strings.Contains(qLower, "call chain") || strings.Contains(qLower, "flow") {
		contract.TaskType = "trace"
		contract.NeedsCallGraph = true
	} else if strings.Contains(qLower, "test") || strings.Contains(qLower, "verify") {
		contract.TaskType = "test"
		contract.NeedsTests = true
	} else if strings.Contains(qLower, "config") || strings.Contains(qLower, "setting") {
		contract.TaskType = "config"
		contract.NeedsConfig = true
	} else {
		contract.TaskType = "lookup"
	}

	// Entity extraction (words with dots, slashes, or CamelCase)
	fields := strings.Fields(query)
	for _, f := range fields {
		fClean := strings.Trim(f, "(),;:\"'[]`")
		if strings.Contains(fClean, "/") || strings.Contains(fClean, ".") || strings.Contains(fClean, "_") {
			contract.Entities = append(contract.Entities, fClean)
		}
	}

	return contract
}

// SufficiencyResult provides the diagnostic evaluation of an evidence set against a query contract (R18 §28).
type SufficiencyResult struct {
	Sufficient      bool     `json:"sufficient"`
	Coverage        float64  `json:"coverage"` // Evidence Coverage Ratio (ECR)
	MissingClaims   []Claim  `json:"missing_claims"`
	MissingEvidence []string `json:"missing_evidence"`
	Conflicts       []string `json:"conflicts"`
	Reason          string   `json:"reason"`
}

// EvaluateSufficiency calculates the Evidence Coverage Ratio (ECR) and determines whether
// the selected evidence set E is sufficient to answer query q under contract C (R18 §28, §30).
//
// Relevance != Sufficiency (R18 §31).
func EvaluateSufficiency(evidence []*EvidenceNode, contract QueryContract, minCoverage float64) SufficiencyResult {
	if minCoverage <= 0 {
		minCoverage = 0.8
	}

	evidencePaths := make(map[string]bool)
	evidenceSymbols := make(map[string]bool)
	var conflictIDs []string

	for _, e := range evidence {
		evidencePaths[strings.ToLower(filepath.ToSlash(e.Path))] = true
		if e.Symbol != "" {
			evidenceSymbols[strings.ToLower(e.Symbol)] = true
		}
		// Look for explicit conflict signals in evidence
		if strings.Contains(strings.ToLower(e.Content), "conflict") || strings.Contains(strings.ToLower(e.Content), "contradict") {
			conflictIDs = append(conflictIDs, e.ID)
		}
	}

	// 1. Check Required Evidence
	var missingEvidence []string
	for _, req := range contract.RequiredEvidence {
		reqNorm := strings.ToLower(filepath.ToSlash(req))
		found := false
		for p := range evidencePaths {
			if p == reqNorm || strings.HasSuffix(p, "/"+reqNorm) || strings.HasPrefix(p, reqNorm) {
				found = true
				break
			}
		}
		if !found {
			missingEvidence = append(missingEvidence, req)
		}
	}

	// 2. Check Claims & calculate ECR
	totalWeight := 0.0
	supportedWeight := 0.0
	var missingClaims []Claim

	for _, claim := range contract.RequiredClaims {
		w := claim.Weight
		if w <= 0 {
			w = 1.0
		}
		totalWeight += w

		// Check if evidence contains claim entities or text keywords
		claimWords := strings.Fields(strings.ToLower(claim.Text))
		matchCount := 0
		meaningfulWords := 0
		for _, w := range claimWords {
			wClean := strings.Trim(w, ",.;:\"'()")
			if len(wClean) > 3 {
				meaningfulWords++
				matched := false
				for _, e := range evidence {
					if strings.Contains(strings.ToLower(e.Content), wClean) || strings.Contains(strings.ToLower(e.Path), wClean) {
						matched = true
						break
					}
				}
				if matched {
					matchCount++
				}
			}
		}

		if meaningfulWords > 0 && float64(matchCount)/float64(meaningfulWords) >= 0.5 {
			supportedWeight += w
		} else if meaningfulWords == 0 {
			supportedWeight += w
		} else {
			missingClaims = append(missingClaims, claim)
		}
	}

	coverage := 1.0
	if totalWeight > 0 {
		coverage = supportedWeight / totalWeight
	} else if len(contract.RequiredEvidence) > 0 {
		coveredReqs := len(contract.RequiredEvidence) - len(missingEvidence)
		coverage = float64(coveredReqs) / float64(len(contract.RequiredEvidence))
	}

	sufficient := coverage >= minCoverage && len(missingEvidence) == 0 && len(conflictIDs) == 0
	var reason string
	if !sufficient {
		var parts []string
		if len(missingEvidence) > 0 {
			parts = append(parts, fmt.Sprintf("missing %d required evidence nodes (%v)", len(missingEvidence), missingEvidence))
		}
		if len(missingClaims) > 0 {
			parts = append(parts, fmt.Sprintf("coverage %.1f%% below threshold %.1f%% (missing %d claims)", coverage*100, minCoverage*100, len(missingClaims)))
		}
		if len(conflictIDs) > 0 {
			parts = append(parts, fmt.Sprintf("detected %d conflicting evidence nodes", len(conflictIDs)))
		}
		reason = strings.Join(parts, "; ")
	} else {
		reason = fmt.Sprintf("sufficient evidence: coverage %.1f%%, all %d required evidence items present", coverage*100, len(contract.RequiredEvidence))
	}

	return SufficiencyResult{
		Sufficient:      sufficient,
		Coverage:        coverage,
		MissingClaims:   missingClaims,
		MissingEvidence: missingEvidence,
		Conflicts:       conflictIDs,
		Reason:          reason,
	}
}
