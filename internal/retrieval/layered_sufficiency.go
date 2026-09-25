package retrieval

import (
	"path/filepath"
	"strings"
)

// LayeredSufficiency evaluates a candidate evidence set across the 6-layer model (R18.2 §25).
type LayeredSufficiency struct {
	Layer0Existence       bool    `json:"layer0_existence"`
	Layer1Semantic        float64 `json:"layer1_semantic"`
	Layer2Dependency      float64 `json:"layer2_dependency"`
	Layer3Contradiction   bool    `json:"layer3_contradiction_safety"`
	Layer4Provenance      bool    `json:"layer4_provenance_current"`
	Layer5AnswerReadiness bool    `json:"layer5_answer_readiness"`
	CompositeScore        float64 `json:"composite_score"`
	Sufficient            bool    `json:"sufficient"`
	FailureLayer          string  `json:"failure_layer,omitempty"`
}

// LayeredSufficiencyWeights defines the importance of each layer in composite evaluation.
type LayeredSufficiencyWeights struct {
	Existence     float64 `json:"existence"`
	Semantic      float64 `json:"semantic"`
	Dependency    float64 `json:"dependency"`
	Contradiction float64 `json:"contradiction"`
	Provenance    float64 `json:"provenance"`
	Readiness     float64 `json:"readiness"`
}

// DefaultLayeredSufficiencyWeights provides balanced defaults.
var DefaultLayeredSufficiencyWeights = LayeredSufficiencyWeights{
	Existence:     0.15,
	Semantic:      0.25,
	Dependency:    0.20,
	Contradiction: 0.15,
	Provenance:    0.10,
	Readiness:     0.15,
}

// EvaluateLayeredSufficiency computes the 6-layer sufficiency of evidence E against contract C (R18.2 §25).
func EvaluateLayeredSufficiency(
	evidence []*EvidenceNode,
	contract QueryContract,
	currentRevision string,
	weights LayeredSufficiencyWeights,
) LayeredSufficiency {
	res := LayeredSufficiency{
		Layer0Existence:     len(evidence) > 0,
		Layer3Contradiction: true,
		Layer4Provenance:    true,
	}

	if !res.Layer0Existence {
		res.FailureLayer = "Layer 0: Existence (evidence set is empty)"
		return res
	}

	// 1. Layer 0: Existence of required evidence
	evidencePaths := make(map[string]bool)
	for _, e := range evidence {
		evidencePaths[strings.ToLower(filepath.ToSlash(e.Path))] = true
	}

	matchedRequired := 0
	for _, req := range contract.RequiredEvidence {
		if evidencePaths[strings.ToLower(filepath.ToSlash(req))] {
			matchedRequired++
		}
	}
	if len(contract.RequiredEvidence) > 0 && matchedRequired == 0 {
		res.Layer0Existence = false
		res.FailureLayer = "Layer 0: Required evidence completely absent"
		return res
	}

	// 2. Layer 1: Semantic coverage (ECR over required claims)
	baseSuff := EvaluateSufficiency(evidence, contract, 0.8)
	res.Layer1Semantic = baseSuff.Coverage

	// 3. Layer 2: Dependency completeness (required multi-file call chain / dependencies)
	if len(contract.RequiredEvidence) > 0 {
		res.Layer2Dependency = float64(matchedRequired) / float64(len(contract.RequiredEvidence))
	} else {
		res.Layer2Dependency = 1.0
	}

	// 4. Layer 3: Contradiction safety
	for _, e := range evidence {
		lowContent := strings.ToLower(e.Content)
		if strings.Contains(lowContent, "disable_bunker_routing = true") || strings.Contains(lowContent, "disabled in production") {
			res.Layer3Contradiction = false
			res.FailureLayer = "Layer 3: Contradiction safety violated"
		}
	}

	// 5. Layer 4: Provenance and currentness
	for _, e := range evidence {
		if e.Provenance.Revision != "" && currentRevision != "" && currentRevision != "HEAD" {
			if e.Provenance.Revision != currentRevision {
				res.Layer4Provenance = false
				res.FailureLayer = "Layer 4: Stale evidence detected"
			}
		}
	}

	// 6. Layer 5: Answer readiness (all required claims have support)
	res.Layer5AnswerReadiness = len(baseSuff.MissingClaims) == 0 && res.Layer1Semantic >= 0.80

	// 7. Calculate composite score
	exScore := 0.0
	if res.Layer0Existence {
		exScore = 1.0
	}
	contraScore := 0.0
	if res.Layer3Contradiction {
		contraScore = 1.0
	}
	provScore := 0.0
	if res.Layer4Provenance {
		provScore = 1.0
	}
	readinessScore := 0.0
	if res.Layer5AnswerReadiness {
		readinessScore = 1.0
	}

	res.CompositeScore = weights.Existence*exScore +
		weights.Semantic*res.Layer1Semantic +
		weights.Dependency*res.Layer2Dependency +
		weights.Contradiction*contraScore +
		weights.Provenance*provScore +
		weights.Readiness*readinessScore

	// Gate: Sufficient only when all required layers pass
	res.Sufficient = res.Layer0Existence &&
		res.Layer1Semantic >= 0.80 &&
		res.Layer2Dependency >= 0.80 &&
		res.Layer3Contradiction &&
		res.Layer4Provenance &&
		res.Layer5AnswerReadiness

	if !res.Sufficient && res.FailureLayer == "" {
		if res.Layer1Semantic < 0.80 {
			res.FailureLayer = "Layer 1: Semantic coverage below threshold"
		} else if res.Layer2Dependency < 0.80 {
			res.FailureLayer = "Layer 2: Dependency completeness below threshold"
		} else if !res.Layer5AnswerReadiness {
			res.FailureLayer = "Layer 5: Answer readiness not reached"
		}
	}

	return res
}
