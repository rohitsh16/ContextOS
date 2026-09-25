package verification

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/retrieval"
)

// ClaimStatus indicates whether an individual assertion is grounded in evidence (R18.1 §40).
type ClaimStatus string

const (
	ClaimSupported    ClaimStatus = "SUPPORTED"
	ClaimPartial      ClaimStatus = "PARTIAL"
	ClaimContradicted ClaimStatus = "CONTRADICTED"
	ClaimUnknown      ClaimStatus = "UNKNOWN"
)

// AtomicClaim represents an extracted, verifiable assertion from an answer (R18.1 §40).
type AtomicClaim struct {
	ID        string      `json:"id"`
	Text      string      `json:"text"`
	Status    ClaimStatus `json:"status"`
	Evidence  []string    `json:"evidence_ids,omitempty"`
	Confidence float64    `json:"confidence"`
	Rationale string      `json:"rationale,omitempty"`
}

// ClaimVerificationResult aggregates claim-level metrics (R18.1 §59).
type ClaimVerificationResult struct {
	TotalClaims          int           `json:"total_claims"`
	SupportedClaims      int           `json:"supported_claims"`
	PartialClaims        int           `json:"partial_claims"`
	ContradictedClaims   int           `json:"contradicted_claims"`
	UnknownClaims        int           `json:"unknown_claims"`
	ClaimPrecision       float64       `json:"claim_precision"`        // CP = supported / total
	UnsupportedClaimRate float64       `json:"unsupported_claim_rate"` // UCR = unsupported / total
	ContradictionRate    float64       `json:"contradiction_rate"`     // CR = contradicted / total
	Claims               []AtomicClaim `json:"claims"`
	LatencyMS            int64         `json:"latency_ms"`
}

var sentenceBoundary = regexp.MustCompile(`[.?!]\s+|\n+`)

// ExtractAtomicClaims parses generated text into verifiable atomic assertions.
func ExtractAtomicClaims(answerText string) []AtomicClaim {
	raw := sentenceBoundary.Split(answerText, -1)
	var claims []AtomicClaim
	idx := 1
	for _, s := range raw {
		clean := strings.TrimSpace(s)
		if len(clean) < 10 {
			continue
		}
		claims = append(claims, AtomicClaim{
			ID:         filepath.Join("claim", string(rune('0'+idx))),
			Text:       clean,
			Status:     ClaimUnknown,
			Confidence: 0.5,
		})
		idx++
	}
	return claims
}

func isNegativeStatement(text string) bool {
	markers := []string{" not ", "no ", "never ", "without ", " don't ", " doesn't ", " cannot ", " isn't ", " aren't ", " won't "}
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}

// SemanticVerifyClaims evaluates each claim against the admitted evidence set (R18.1 §41).
// Note: Lexical overlap is insufficient: "The service does NOT use Kafka" vs "The service uses Kafka"
// has high lexical overlap but is contradictory!
func SemanticVerifyClaims(claims []AtomicClaim, evidence []*retrieval.EvidenceNode) ClaimVerificationResult {
	start := time.Now()
	var verified []AtomicClaim
	supported := 0
	partial := 0
	contradicted := 0
	unknown := 0

	for _, claim := range claims {
		cText := strings.ToLower(claim.Text)
		cNeg := isNegativeStatement(cText)

		var bestStatus ClaimStatus = ClaimUnknown
		var matchedEv []string
		rationale := "no supporting evidence found"
		maxOverlap := 0.0

		for _, e := range evidence {
			eText := strings.ToLower(e.Content)
			eNeg := isNegativeStatement(eText)

			// Check keyword containment
			words := strings.Fields(cText)
			matchCount := 0
			meaningfulWords := 0
			for _, w := range words {
				wClean := strings.Trim(w, ",.;:\"'()")
				if len(wClean) > 3 {
					meaningfulWords++
					if strings.Contains(eText, wClean) || (len(wClean) > 4 && strings.Contains(eText, wClean[:len(wClean)-1])) {
						matchCount++
					}
				}
			}
			overlap := 0.0
			if meaningfulWords > 0 {
				overlap = float64(matchCount) / float64(meaningfulWords)
			}

			if overlap >= 0.4 {
				if overlap > maxOverlap {
					maxOverlap = overlap
				}
				matchedEv = append(matchedEv, e.ID)

				// Semantic polarity check
				if cNeg != eNeg && overlap >= 0.5 {
					bestStatus = ClaimContradicted
					rationale = "semantic contradiction: claim polarity does not match evidence polarity"
					break
				} else if overlap >= 0.6 {
					bestStatus = ClaimSupported
					rationale = "strongly supported by evidence node " + e.ID
				} else if bestStatus != ClaimSupported {
					bestStatus = ClaimPartial
					rationale = "partially supported by evidence node " + e.ID
				}
			}
		}

		switch bestStatus {
		case ClaimSupported:
			supported++
		case ClaimPartial:
			partial++
		case ClaimContradicted:
			contradicted++
		default:
			unknown++
		}

		verified = append(verified, AtomicClaim{
			ID:         claim.ID,
			Text:       claim.Text,
			Status:     bestStatus,
			Evidence:   matchedEv,
			Confidence: maxOverlap,
			Rationale:  rationale,
		})
	}

	total := len(claims)
	cp := 0.0
	ucr := 0.0
	cr := 0.0
	if total > 0 {
		cp = float64(supported) / float64(total)
		ucr = float64(unknown+partial) / float64(total)
		cr = float64(contradicted) / float64(total)
	}

	return ClaimVerificationResult{
		TotalClaims:          total,
		SupportedClaims:      supported,
		PartialClaims:        partial,
		ContradictedClaims:   contradicted,
		UnknownClaims:        unknown,
		ClaimPrecision:       cp,
		UnsupportedClaimRate: ucr,
		ContradictionRate:    cr,
		Claims:               verified,
		LatencyMS:            time.Since(start).Milliseconds(),
	}
}

// VerifyDeterministicRepositoryChecks executes Level 1 deterministic facts (R18.1 §43).
func VerifyDeterministicRepositoryChecks(file string, symbol string, allFiles []gitidx.EvidenceFile) (bool, string) {
	if file != "" {
		found := false
		for _, f := range allFiles {
			if strings.EqualFold(filepath.ToSlash(f.Path), filepath.ToSlash(file)) {
				found = true
				break
			}
		}
		if !found {
			return false, "file does not exist in admissible repository: " + file
		}
	}
	return true, "deterministic check passed"
}
