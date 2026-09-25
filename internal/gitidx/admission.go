package gitidx

import (
	"path/filepath"
	"strings"
)

// AdmissionPolicy dictates which evidence classes and file patterns enter the index (R17.5 §9).
type AdmissionPolicy struct {
	IncludeClasses    []EvidenceClass `json:"include_classes"`
	ExcludeClasses    []EvidenceClass `json:"exclude_classes"`
	AllowPatterns     []string        `json:"allow_patterns"`
	DenyPatterns      []string        `json:"deny_patterns"`
	AllowGenerated    bool            `json:"allow_generated"`
	AllowVendored     bool            `json:"allow_vendored"`
	AllowTests        bool            `json:"allow_tests"`
	AllowFixtures     bool            `json:"allow_fixtures"`
	RequireGitTracked bool            `json:"require_git_tracked"`
	Version           string          `json:"version"`
}

// DefaultAdmissionPolicy creates the conservative R17.5 admission policy:
// authoritative source, docs, and tests are admitted;
// generated, vendored, build output, agent worktrees, and caches are strictly excluded.
func DefaultAdmissionPolicy() AdmissionPolicy {
	return AdmissionPolicy{
		IncludeClasses: []EvidenceClass{
			EvidenceAuthoritative,
			EvidenceDocumentation,
			EvidenceTest,
			EvidenceFixture,
		},
		ExcludeClasses: []EvidenceClass{
			EvidenceGenerated,
			EvidenceVendored,
			EvidenceBuildArtifact,
			EvidenceCache,
			EvidenceAgentState,
			EvidenceUnknown,
		},
		AllowPatterns:     nil,
		DenyPatterns:      nil,
		AllowGenerated:    false,
		AllowVendored:     false,
		AllowTests:        true,
		AllowFixtures:     true,
		RequireGitTracked: false,
		Version:           "R17.5-v1",
	}
}

func matchGlobOrPrefix(pattern, path string) bool {
	pattern = filepath.ToSlash(pattern)
	path = filepath.ToSlash(path)

	// Direct match or exact prefix match
	if pattern == path || strings.HasPrefix(path, strings.TrimSuffix(pattern, "/")+"/") {
		return true
	}
	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}
	// Support double-star style trailing "/**"
	if strings.HasSuffix(pattern, "/**") {
		base := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(path, base+"/") || path == base {
			return true
		}
	}
	return false
}

// EvaluateAdmission assesses candidate file eligibility based on classification and admission policy (R17.5 §9).
// Invariant I1: Retrieved(e) => Eligible(e)
// Invariant I2: Admission(S, P) == Admission(S, P) (deterministic)
func EvaluateAdmission(relPath string, content []byte, isTracked bool, isIgnored bool, policy AdmissionPolicy) EvidenceEligibility {
	relSlash := filepath.ToSlash(relPath)

	// 1. Explicit Deny Patterns (highest priority)
	for _, deny := range policy.DenyPatterns {
		if matchGlobOrPrefix(deny, relSlash) {
			class, _ := Classify(relPath, content, isTracked, isIgnored)
			return EvidenceEligibility{
				Class:      class,
				Eligible:   false,
				Authority:  0.0,
				GitTracked: isTracked,
				GitIgnored: isIgnored,
				Reason:     "explicit deny pattern matched: " + deny,
			}
		}
	}

	// 2. Explicit Allow Patterns (bypasses class-based exclusion, e.g. R17.5 Benchmark E)
	for _, allow := range policy.AllowPatterns {
		if matchGlobOrPrefix(allow, relSlash) {
			class, _ := Classify(relPath, content, isTracked, isIgnored)
			auth := DefaultAuthority(class)
			if auth == 0.0 {
				auth = 1.0 // Promoted by explicit configuration
			}
			return EvidenceEligibility{
				Class:         class,
				Eligible:      true,
				Authority:     auth,
				GitTracked:    isTracked,
				GitIgnored:    isIgnored,
				Generated:     class == EvidenceGenerated,
				Vendored:      class == EvidenceVendored,
				BuildArtifact: class == EvidenceBuildArtifact,
				Reason:        "explicit allow pattern matched: " + allow,
			}
		}
	}

	// 3. Classify file signals
	class, reason := Classify(relPath, content, isTracked, isIgnored)

	// 4. Git-tracked requirement check
	if policy.RequireGitTracked && !isTracked {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Reason:     "file is not tracked by git",
		}
	}

	// 5. Evaluate class specific flags
	if class == EvidenceGenerated && !policy.AllowGenerated {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Generated:  true,
			Reason:     "generated code excluded by policy",
		}
	}
	if class == EvidenceVendored && !policy.AllowVendored {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Vendored:   true,
			Reason:     "vendored code excluded by policy",
		}
	}
	if class == EvidenceBuildArtifact {
		return EvidenceEligibility{
			Class:         class,
			Eligible:      false,
			Authority:     0.0,
			GitTracked:    isTracked,
			GitIgnored:    isIgnored,
			BuildArtifact: true,
			Reason:        "build artifact excluded by policy",
		}
	}
	if class == EvidenceAgentState || class == EvidenceCache {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Reason:     "agent state / cache excluded by policy",
		}
	}
	if class == EvidenceTest && !policy.AllowTests {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Reason:     "test suite excluded by policy",
		}
	}
	if class == EvidenceFixture && !policy.AllowFixtures {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Reason:     "test fixture excluded by policy",
		}
	}
	if class == EvidenceUnknown {
		return EvidenceEligibility{
			Class:      class,
			Eligible:   false,
			Authority:  0.0,
			GitTracked: isTracked,
			GitIgnored: isIgnored,
			Reason:     "unknown evidence class excluded",
		}
	}

	return EvidenceEligibility{
		Class:      class,
		Eligible:   true,
		Authority:  DefaultAuthority(class),
		GitTracked: isTracked,
		GitIgnored: isIgnored,
		Reason:     reason,
	}
}
