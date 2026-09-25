package gitidx

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// EvidenceProvenance records immutable lineage, revision, and authority for every indexed item (R17.5 §6).
type EvidenceProvenance struct {
	RepoID        string        `json:"repo_id"`
	Revision      string        `json:"revision"`
	Path          string        `json:"path"`
	StartLine     int           `json:"start_line"`
	EndLine       int           `json:"end_line"`
	ContentHash   string        `json:"content_hash"`
	SourceClass   EvidenceClass `json:"source_class"`
	Eligible      bool          `json:"eligible"`
	Authority     float64       `json:"authority"`
	GitTracked    bool          `json:"git_tracked"`
	GitIgnored    bool          `json:"git_ignored"`
	Generated     bool          `json:"generated"`
	Vendored      bool          `json:"vendored"`
	IndexedAt     time.Time     `json:"indexed_at"`
	PolicyVersion string        `json:"policy_version"`
}

// ComputeContentHash returns the hex-encoded SHA-256 of the given content.
func ComputeContentHash(content []byte) string {
	h := sha256.Sum256(content)
	return hex.EncodeToString(h[:])
}

// NewEvidenceProvenance constructs a provenance record for an admissible source or symbol.
func NewEvidenceProvenance(
	repoID, revision, relPath string,
	startLine, endLine int,
	contentHash string,
	eligibility EvidenceEligibility,
	policyVersion string,
) EvidenceProvenance {
	return EvidenceProvenance{
		RepoID:        repoID,
		Revision:      revision,
		Path:          relPath,
		StartLine:     startLine,
		EndLine:       endLine,
		ContentHash:   contentHash,
		SourceClass:   eligibility.Class,
		Eligible:      eligibility.Eligible,
		Authority:     eligibility.Authority,
		GitTracked:    eligibility.GitTracked,
		GitIgnored:    eligibility.GitIgnored,
		Generated:     eligibility.Generated,
		Vendored:      eligibility.Vendored,
		IndexedAt:     time.Now().UTC(),
		PolicyVersion: policyVersion,
	}
}
