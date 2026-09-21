package state

import "time"

// EvidenceRequirement specifies an empirical condition or fact needed to validate a decision.
type EvidenceRequirement struct {
	ID          string  `json:"id"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`    // Criticality weight (e.g. 1.0 = normal, 3.0 = security/critical)
	Satisfied   bool    `json:"satisfied"`
	EvidenceID  string  `json:"evidence_id,omitempty"`
}

// Evidence represents an empirical observation from code, test, compiler, or user.
type Evidence struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"` // "file", "test", "compiler", "git", "user"
	Location  string    `json:"location,omitempty"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Weight    float64   `json:"weight"`
	Conflict  bool      `json:"conflict,omitempty"`
}

// EvidenceState tracks required vs acquired evidence, weighted coverage, and conflict metrics.
type EvidenceState struct {
	Required []EvidenceRequirement `json:"required"`
	Acquired []Evidence            `json:"acquired"`
	Missing  []EvidenceRequirement `json:"missing"`

	Coverage float64 `json:"coverage"` // [0.0, 1.0] weighted coverage
	Conflict float64 `json:"conflict"` // [0.0, 1.0] ratio of conflicting observations
}

// NewEvidenceState creates an evidence tracker with initial requirements.
func NewEvidenceState(reqs []EvidenceRequirement) EvidenceState {
	es := EvidenceState{
		Required: reqs,
		Acquired: make([]Evidence, 0),
		Missing:  make([]EvidenceRequirement, 0),
	}
	es.Recompute()
	return es
}

// AddEvidence ingests a new piece of evidence and links it to matching requirements.
func (es *EvidenceState) AddEvidence(ev Evidence) {
	es.Acquired = append(es.Acquired, ev)
	es.Recompute()
}

// Recompute recalculates weighted coverage, missing requirements, and conflict score.
func (es *EvidenceState) Recompute() {
	if len(es.Required) == 0 {
		es.Coverage = 1.0
		es.Conflict = 0.0
		es.Missing = nil
		return
	}

	var totalWeight float64
	var satisfiedWeight float64
	es.Missing = make([]EvidenceRequirement, 0)

	// Check satisfied requirements against acquired evidence
	for _, req := range es.Required {
		totalWeight += req.Weight
		satisfied := false
		for _, ev := range es.Acquired {
			if ev.Weight > 0 && !ev.Conflict {
				// If requirement ID matches or evidence explicitly satisfies it
				if req.EvidenceID != "" && req.EvidenceID == ev.ID {
					satisfied = true
					break
				}
			}
		}
		if req.Satisfied {
			satisfied = true
		}

		if satisfied {
			satisfiedWeight += req.Weight
		} else {
			es.Missing = append(es.Missing, req)
		}
	}

	if totalWeight > 0 {
		es.Coverage = satisfiedWeight / totalWeight
	} else {
		es.Coverage = 1.0
	}

	// Calculate conflict ratio
	if len(es.Acquired) > 0 {
		var conflictCount int
		for _, ev := range es.Acquired {
			if ev.Conflict {
				conflictCount++
			}
		}
		es.Conflict = float64(conflictCount) / float64(len(es.Acquired))
	} else {
		es.Conflict = 0.0
	}
}
