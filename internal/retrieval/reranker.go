package retrieval

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
)

// TaskAwareRerank rescores candidates using task intent, entity alignment, identifier presence, and graph proximity (R18.1 §14).
func TaskAwareRerank(
	candidates []Candidate,
	rep *QueryRepresentation,
	profile TaskRoutingProfile,
	targetLimit int,
) []Candidate {
	if len(candidates) == 0 {
		return candidates
	}

	for i := range candidates {
		c := &candidates[i]
		if c.Trace == nil {
			c.Trace = &CandidateTrace{
				ID:         c.ID,
				Path:       c.Path,
				Name:       c.Name,
				Stages:     []RetrievalStage{},
				Admissible: true,
			}
		}
		c.Trace.AddStage(StageRerank)

		baseScore := c.Score

		// 1. Entity alignment bonus: checks if candidate content or path contains high-salience query entities
		entityBonus := 0.0
		lowPath := strings.ToLower(c.Path)
		lowContent := strings.ToLower(c.Content)
		for _, ent := range rep.Entities {
			entLow := strings.ToLower(ent.Name)
			if isNegatedTerm(entLow, rep.Negations) {
				continue
			}
			if strings.Contains(lowPath, entLow) {
				entityBonus += 0.35 * ent.Salience
			} else if strings.Contains(lowContent, entLow) {
				entityBonus += 0.20 * ent.Salience
			}
		}

		// 2. Exact identifier match bonus
		identBonus := 0.0
		for _, id := range rep.Identifiers {
			idLow := strings.ToLower(id)
			if isNegatedTerm(idLow, rep.Negations) {
				continue
			}
			if strings.Contains(lowPath, idLow) || strings.EqualFold(filepath.Base(c.Path), id) {
				identBonus += 0.30
			} else if strings.Contains(lowContent, idLow) {
				identBonus += 0.15
			}
		}

		// 3. Artifact type alignment bonus (e.g. query asked for "policy", candidate is in "plan/policy/...")
		artBonus := 0.0
		for _, art := range rep.ArtifactTypes {
			if strings.Contains(lowPath, art.Type) {
				artBonus += 0.25
			}
		}

		// 4. Test file penalty/boost based on query intent
		testMod := 0.0
		isTest := strings.HasSuffix(c.Path, "_test.go") || strings.Contains(c.Path, "/test/")
		if isTest {
			if rep.Intent == QueryIntentTest {
				testMod = +0.20
			} else {
				testMod = -0.15 // penalize test files if not looking for tests
			}
		}

		// 5. Authority weighting (authoritative source vs tests vs fixtures vs distractors)
		authorityScore := 0.8
		if isTest {
			authorityScore = 0.5
		} else if strings.Contains(c.Path, "/mock/") || strings.Contains(c.Path, "distractor") || strings.Contains(c.Path, "override") {
			authorityScore = 0.3
		} else {
			authorityScore = 1.0
		}

		// Compute final task-aware reranked score
		rerankedScore := baseScore + entityBonus + identBonus + artBonus + testMod + (profile.AuthorityWeight * authorityScore)

		// Negation penalty on candidates matching negated concepts in path/name
		for _, neg := range rep.Negations {
			negLow := strings.ToLower(neg)
			if len(negLow) < 3 || isNegationWord(negLow) || isCommonStopWord(negLow) {
				continue
			}
			if strings.Contains(lowPath, negLow) || strings.Contains(strings.ToLower(c.Name), negLow) {
				rerankedScore *= 0.05
				break
			}
		}

		c.Score = rerankedScore
		c.Trace.FinalScore = rerankedScore
	}

	// Sort descending by reranked score with deterministic stability tie-breaking
	sort.Slice(candidates, func(i, j int) bool {
		diff := candidates[i].Score - candidates[j].Score
		if math.Abs(diff) > 1e-6 {
			return diff > 0
		}
		// Deterministic stability tie-breaker:
		return candidates[i].Path < candidates[j].Path
	})

	for i := range candidates {
		candidates[i].Trace.Rank = i + 1
	}

	if targetLimit > 0 && len(candidates) > targetLimit {
		candidates = candidates[:targetLimit]
	}

	return candidates
}

func isNegationWord(w string) bool {
	switch w {
	case "not", "never", "no", "stop", "stops", "prevent", "prevents",
		"exclude", "excludes", "excluding", "block", "blocks", "blocking",
		"without", "disable", "disabled", "deny", "denies", "a", "an", "the":
		return true
	}
	return false
}

func isNegatedTerm(word string, negations []string) bool {
	for _, n := range negations {
		nLow := strings.ToLower(n)
		if len(nLow) < 3 || isNegationWord(nLow) || isCommonStopWord(nLow) {
			continue
		}
		if strings.EqualFold(word, nLow) || strings.Contains(word, nLow) {
			return true
		}
	}
	return false
}
