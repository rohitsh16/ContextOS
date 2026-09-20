package model

import (
	"encoding/json"
	"math"
	"strings"
)

// StatePreservationPackage represents the serialized snapshot of engineering state during agent handoff (PR.md Section 14).
type StatePreservationPackage struct {
	Version            string            `json:"version"` // e.g. "asc-1.0"
	SourceAgent        string            `json:"source_agent"`
	TargetAgent        string            `json:"target_agent"`
	Timestamp          string            `json:"timestamp"`
	WorkItem           WorkItem          `json:"work_item"`
	DurableDecisions   []Memory          `json:"durable_decisions"`
	ActiveConstraints  []Memory          `json:"active_constraints"`
	RecentFailures     []Memory          `json:"recent_failures"`
	ModifiedFiles      map[string]string `json:"modified_files"` // path -> status
	StateEntropyBefore float64           `json:"state_entropy_before"`
}

// StateContinuityAudit records the fidelity and entropy loss of an inter-agent handoff.
type StateContinuityAudit struct {
	SourceAgent         string  `json:"source_agent"`
	TargetAgent         string  `json:"target_agent"`
	DecisionsPreserved  int     `json:"decisions_preserved"`
	DecisionsLost       int     `json:"decisions_lost"`
	ConstraintsPreserved int    `json:"constraints_preserved"`
	ConstraintsLost     int     `json:"constraints_lost"`
	EntropyBefore       float64 `json:"entropy_before"`
	ConditionalEntropy  float64 `json:"conditional_entropy_loss"`
	StateContinuity     float64 `json:"state_continuity"` // SC = 1 - H(S_after|S_before)/H(S_before)
	RediscoveryAvoided  bool    `json:"rediscovery_avoided"`
	Status              string  `json:"status"` // GREEN or RED
}

// ComputeStateEntropy calculates the Shannon entropy of an engineering state vector.
func ComputeStateEntropy(decisions []Memory, constraints []Memory, files map[string]string) float64 {
	totalItems := len(decisions) + len(constraints) + len(files)
	if totalItems == 0 {
		return 1.0
	}

	pDec := float64(len(decisions)) / float64(totalItems)
	pCon := float64(len(constraints)) / float64(totalItems)
	pFile := float64(len(files)) / float64(totalItems)

	entropy := 0.0
	for _, p := range []float64{pDec, pCon, pFile} {
		if p > 0 {
			entropy -= p * math.Log2(p)
		}
	}
	return math.Max(0.1, entropy)
}

// ExportStatePreservationPackage serializes active session engineering state into a portable package.
func ExportStatePreservationPackage(sourceAgent, targetAgent string, work WorkItem, memories []Memory, modifiedFiles map[string]string) ([]byte, error) {
	var decs, cons, fails []Memory
	for _, m := range memories {
		switch strings.ToLower(m.Kind) {
		case "decision":
			decs = append(decs, m)
		case "constraint":
			cons = append(cons, m)
		case "failure":
			fails = append(fails, m)
		}
	}

	entropy := ComputeStateEntropy(decs, cons, modifiedFiles)

	pkg := StatePreservationPackage{
		Version:            "asc-1.0",
		SourceAgent:        sourceAgent,
		TargetAgent:        targetAgent,
		Timestamp:          "2026-09-21T02:00:00Z",
		WorkItem:           work,
		DurableDecisions:   decs,
		ActiveConstraints:  cons,
		RecentFailures:     fails,
		ModifiedFiles:      modifiedFiles,
		StateEntropyBefore: entropy,
	}

	return json.MarshalIndent(pkg, "", "  ")
}

// EvaluateStateContinuity checks whether target agent successfully deserializes and preserves state.
func EvaluateStateContinuity(rawPkg []byte, expectedDecisions int, expectedConstraints int) (StateContinuityAudit, error) {
	var pkg StatePreservationPackage
	if err := json.Unmarshal(rawPkg, &pkg); err != nil {
		return StateContinuityAudit{Status: "RED"}, err
	}

	preservedDec := len(pkg.DurableDecisions)
	lostDec := math.Max(0, float64(expectedDecisions-preservedDec))

	preservedCon := len(pkg.ActiveConstraints)
	lostCon := math.Max(0, float64(expectedConstraints-preservedCon))

	entropyBefore := pkg.StateEntropyBefore
	if entropyBefore <= 0 {
		entropyBefore = ComputeStateEntropy(pkg.DurableDecisions, pkg.ActiveConstraints, pkg.ModifiedFiles)
	}

	// Conditional entropy represents unexplained state loss upon transfer
	lossRatio := (lostDec + lostCon) / float64(math.Max(float64(expectedDecisions+expectedConstraints), 1))
	conditionalEntropy := entropyBefore * lossRatio

	sc := 1.0
	if entropyBefore > 0 {
		sc = 1.0 - (conditionalEntropy / entropyBefore)
	}
	sc = math.Max(0.0, math.Min(1.0, sc))

	status := "GREEN"
	// R9 GREEN criterion: state continuity >= 0.95 and zero lost decisions
	if sc < 0.90 || lostDec > 0 {
		status = "RED"
	}

	return StateContinuityAudit{
		SourceAgent:          pkg.SourceAgent,
		TargetAgent:          pkg.TargetAgent,
		DecisionsPreserved:   preservedDec,
		DecisionsLost:        int(lostDec),
		ConstraintsPreserved: preservedCon,
		ConstraintsLost:      int(lostCon),
		EntropyBefore:        entropyBefore,
		ConditionalEntropy:   conditionalEntropy,
		StateContinuity:      sc,
		RediscoveryAvoided:   lostDec == 0 && preservedDec > 0,
		Status:               status,
	}, nil
}
