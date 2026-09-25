package gitidx

import (
	"fmt"
	"sort"
	"strings"
)

// AdmissionAuditManifest provides an empirical audit of all candidate files considered during indexing (R17.5 §10).
type AdmissionAuditManifest struct {
	FilesConsidered  int                   `json:"files_considered"`
	ClassCounts      map[EvidenceClass]int `json:"class_counts"`
	AdmittedCount    int                   `json:"admitted_count"`
	RejectedCount    int                   `json:"rejected_count"`
	RejectionReasons map[string]int        `json:"rejection_reasons"`
	DurationMs       float64               `json:"duration_ms"`
	PolicyVersion    string                `json:"policy_version"`
}

// NewAdmissionAuditManifest initializes an empty audit manifest.
func NewAdmissionAuditManifest(policyVersion string) *AdmissionAuditManifest {
	return &AdmissionAuditManifest{
		ClassCounts:      make(map[EvidenceClass]int),
		RejectionReasons: make(map[string]int),
		PolicyVersion:    policyVersion,
	}
}

// Record observes an individual file evaluation.
func (m *AdmissionAuditManifest) Record(eligibility EvidenceEligibility) {
	m.FilesConsidered++
	m.ClassCounts[eligibility.Class]++
	if eligibility.Eligible {
		m.AdmittedCount++
	} else {
		m.RejectedCount++
		reason := eligibility.Reason
		if reason == "" {
			reason = string(eligibility.Class)
		}
		m.RejectionReasons[reason]++
	}
}

// FormatAudit returns the human-readable text audit required by R17.5 §10.
func (m *AdmissionAuditManifest) FormatAudit() string {
	var sb strings.Builder
	sb.WriteString("INDEX AUDIT\n===========\n\n")
	sb.WriteString(fmt.Sprintf("Files considered:        %d\n\n", m.FilesConsidered))

	classes := []EvidenceClass{
		EvidenceAuthoritative,
		EvidenceDocumentation,
		EvidenceTest,
		EvidenceFixture,
		EvidenceGenerated,
		EvidenceVendored,
		EvidenceBuildArtifact,
		EvidenceCache,
		EvidenceAgentState,
		EvidenceUnknown,
	}
	for _, c := range classes {
		if count := m.ClassCounts[c]; count > 0 {
			label := strings.Title(strings.ReplaceAll(string(c), "_", " "))
			sb.WriteString(fmt.Sprintf("%-24s %d\n", label+":", count))
		}
	}
	sb.WriteString(fmt.Sprintf("\nAdmitted:                %d\n", m.AdmittedCount))
	sb.WriteString(fmt.Sprintf("Rejected:                %d\n\n", m.RejectedCount))

	if len(m.RejectionReasons) > 0 {
		sb.WriteString("Top rejection reasons:\n")
		type reasonEntry struct {
			reason string
			count  int
		}
		var list []reasonEntry
		for r, c := range m.RejectionReasons {
			list = append(list, reasonEntry{reason: r, count: c})
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].count > list[j].count
		})
		for i, re := range list {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("  %-22s %d\n", re.reason, re.count))
		}
	}

	return sb.String()
}

// GenerateAdmissionAuditManifest walks the repo and evaluates admission for all candidate files.
func GenerateAdmissionAuditManifest(repoDir string, policy AdmissionPolicy) (*AdmissionAuditManifest, error) {
	_, manifest, err := ListEvidenceFiles(repoDir, policy, "repo", "HEAD")
	if err != nil {
		return nil, err
	}
	return manifest, nil
}

// FormatAuditManifest formats an admission audit manifest for human display.
func FormatAuditManifest(m *AdmissionAuditManifest) string {
	if m == nil {
		return ""
	}
	return m.FormatAudit()
}
