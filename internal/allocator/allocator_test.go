package allocator

import (
	"math"
	"testing"

	"contextos/internal/model"
)

// ─── helpers ────────────────────────────────────────────────────────────────

// ids returns the IDs of all selected candidates in order.
func ids(p model.ContextPlan) []string {
	out := make([]string, len(p.Selected))
	for i, c := range p.Selected {
		out[i] = c.ID
	}
	return out
}

// hasID reports whether id appears anywhere in the selection.
func hasID(p model.ContextPlan, id string) bool {
	for _, c := range p.Selected {
		if c.ID == id {
			return true
		}
	}
	return false
}

// ─── original tests (regression) ────────────────────────────────────────────

func TestPlanRejectsInvalidAndLowAuthority(t *testing.T) {
	req := Request{Task: "kafka transaction", Budget: 100}
	ms := []model.Memory{
		{ID: "good", Kind: "decision", Content: "Use outbox for kafka transaction", Authority: "user", TokenCost: 8},
		{ID: "stale", Kind: "decision", Content: "Old kafka transaction design", Authority: "user", InvalidatedAtRevision: "deadbeef", TokenCost: 8},
		{ID: "weak", Kind: "fact", Content: "Kafka transaction detail", Authority: "inference", TokenCost: 8},
	}
	p := Plan(req, ms)
	if len(p.Selected) != 1 || p.Selected[0].ID != "good" {
		t.Fatalf("expected only 'good' selected, got %v", ids(p))
	}
}

func TestPlanRespectsBudget(t *testing.T) {
	req := Request{Task: "payment kafka", Budget: 10}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "payment kafka decision", Authority: "user", TokenCost: 6},
		{ID: "b", Kind: "decision", Content: "payment kafka failure", Authority: "user", TokenCost: 6},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > 10 {
		t.Fatalf("budget exceeded: %d", p.SelectedTokens)
	}
}

func TestBM25LexicalBoostHighRelevance(t *testing.T) {
	req := Request{Task: "kafka transaction outbox pattern", Budget: 500}
	ms := []model.Memory{
		{ID: "hi", Kind: "decision", Content: "Use kafka transaction outbox pattern for exactly-once delivery", Authority: "user", TokenCost: 40},
		{ID: "lo", Kind: "decision", Content: "Use redis caching for user session data", Authority: "user", TokenCost: 40},
	}
	p := Plan(req, ms)
	if len(p.Selected) < 2 {
		t.Fatalf("expected both selected, got %v", ids(p))
	}
	if p.Selected[0].ID != "hi" {
		t.Errorf("high-overlap item should rank first; got %s", p.Selected[0].ID)
	}
}

func TestSingletonRescueRecoversBudget(t *testing.T) {
	req := Request{Task: "kafka transaction critical outbox", Budget: 100}
	ms := []model.Memory{
		{ID: "big", Kind: "decision", Content: "kafka transaction critical outbox exactly once delivery guarantee", Authority: "user", TokenCost: 95},
		{ID: "s1", Kind: "observation", Content: "network packet loss detected", Authority: "commit", TokenCost: 50},
		{ID: "s2", Kind: "observation", Content: "cpu idle percentage high", Authority: "commit", TokenCost: 60},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > req.Budget {
		t.Errorf("budget exceeded: %d > %d", p.SelectedTokens, req.Budget)
	}
	if len(p.Selected) == 1 && p.Selected[0].ID != "big" {
		t.Errorf("singleton rescue should have selected 'big'; got %s (score=%.5f)", p.Selected[0].ID, p.Selected[0].Score)
	}
}

// ─── edge cases ─────────────────────────────────────────────────────────────

func TestEmptyMemories(t *testing.T) {
	p := Plan(Request{Task: "kafka", Budget: 1000}, nil)
	if len(p.Selected) != 0 {
		t.Errorf("expected 0 selected for empty memories, got %d", len(p.Selected))
	}
	if p.SelectedTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", p.SelectedTokens)
	}
}

func TestZeroBudgetDefaultsTo4000(t *testing.T) {
	p := Plan(Request{Task: "kafka", Budget: 0}, []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka outbox", Authority: "user", TokenCost: 3999},
	})
	// Budget=0 should default to 4000; item with 3999 tokens must be selected.
	if p.Budget != 4000 {
		t.Errorf("expected Budget=4000, got %d", p.Budget)
	}
	if !hasID(p, "a") {
		t.Errorf("3999-token item should fit in 4000-token default budget")
	}
}

func TestNegativeBudgetDefaultsTo4000(t *testing.T) {
	p := Plan(Request{Task: "kafka", Budget: -1}, []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka outbox", Authority: "user", TokenCost: 10},
	})
	if p.Budget != 4000 {
		t.Errorf("negative budget should default to 4000, got %d", p.Budget)
	}
}

func TestAllMemoriesHardRejected(t *testing.T) {
	req := Request{Task: "kafka", Budget: 500}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka outbox", Authority: "user", InvalidatedAtRevision: "abc", TokenCost: 10},
		{ID: "b", Kind: "fact", Content: "kafka fact", Authority: "inference", TokenCost: 10},
		{ID: "c", Kind: "decision", Content: "kafka constraint", Authority: "user", InvalidatedAtRevision: "def", TokenCost: 10},
	}
	p := Plan(req, ms)
	if len(p.Selected) != 0 {
		t.Errorf("all hard-rejected: expected 0 selected, got %v", ids(p))
	}
}

func TestBudgetExactlyMet(t *testing.T) {
	req := Request{Task: "kafka transaction", Budget: 10}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka transaction a", Authority: "user", TokenCost: 5},
		{ID: "b", Kind: "decision", Content: "kafka transaction b", Authority: "user", TokenCost: 5},
	}
	p := Plan(req, ms)
	if p.SelectedTokens != 10 {
		t.Errorf("expected exactly 10 tokens used, got %d", p.SelectedTokens)
	}
	if len(p.Selected) != 2 {
		t.Errorf("expected both selected, got %v", ids(p))
	}
}

func TestSingleCandidateFits(t *testing.T) {
	p := Plan(Request{Task: "kafka transaction", Budget: 100}, []model.Memory{
		{ID: "only", Kind: "decision", Content: "kafka transaction outbox", Authority: "user", TokenCost: 50},
	})
	if !hasID(p, "only") {
		t.Errorf("single fitting item should be selected")
	}
	if p.SelectedTokens != 50 {
		t.Errorf("expected 50 tokens, got %d", p.SelectedTokens)
	}
}

func TestSingleCandidateExceedsBudget(t *testing.T) {
	p := Plan(Request{Task: "kafka transaction", Budget: 20}, []model.Memory{
		{ID: "toobig", Kind: "decision", Content: "kafka transaction outbox", Authority: "user", TokenCost: 50},
	})
	if hasID(p, "toobig") {
		t.Errorf("item exceeding budget should not be selected")
	}
	if p.SelectedTokens != 0 {
		t.Errorf("expected 0 tokens used, got %d", p.SelectedTokens)
	}
}

// ─── scoring correctness ─────────────────────────────────────────────────────

func TestHardStaleAlwaysExcluded(t *testing.T) {
	req := Request{Task: "kafka", Budget: 1000}
	ms := []model.Memory{
		{ID: "stale", Kind: "decision", Content: "kafka outbox", Authority: "user", InvalidatedAtRevision: "oldrev", TokenCost: 10},
		{ID: "fresh", Kind: "decision", Content: "kafka outbox", Authority: "user", TokenCost: 10},
	}
	p := Plan(req, ms)
	if hasID(p, "stale") {
		t.Error("explicitly invalidated memory must never be selected")
	}
	if !hasID(p, "fresh") {
		t.Error("fresh memory should be selected")
	}
}

func TestRevisionFreshnessPenalty(t *testing.T) {
	// An item from a different revision gets a freshness penalty (0.25) but is NOT hard-rejected.
	req := Request{Task: "kafka transaction", Budget: 1000, RepoRevision: "current"}
	ms := []model.Memory{
		{ID: "old-rev", Kind: "decision", Content: "kafka transaction", Authority: "user", ValidFromRevision: "ancient", TokenCost: 10},
		{ID: "fresh", Kind: "decision", Content: "kafka transaction", Authority: "user", ValidFromRevision: "current", TokenCost: 10},
	}
	p := Plan(req, ms)
	// Both should be selected (budget allows), but "fresh" should rank first.
	if len(p.Selected) < 1 {
		t.Fatal("expected at least one selected")
	}
	// Freshness of old-rev must be 0.75 (freshRisk=0.25).
	var oldRevFreshness float64
	for _, c := range p.Candidates {
		if c.ID == "old-rev" {
			oldRevFreshness = c.Freshness
		}
	}
	if math.Abs(oldRevFreshness-0.75) > 0.01 {
		t.Errorf("old-rev Freshness should be 0.75, got %.4f", oldRevFreshness)
	}
	// "fresh" should outrank "old-rev" in the selection.
	if p.Selected[0].ID != "fresh" {
		t.Errorf("fresher item should rank first; got %s", p.Selected[0].ID)
	}
}

func TestConfidenceWeighting(t *testing.T) {
	// Two items with identical content and authority, but different confidence.
	// The RRF rank difference (2 candidates: 1/61 vs 1/62 per signal ≈ 1.6% gap) is much
	// smaller than the confidence difference (0.95 vs 0.30 = 3.2x), so high-conf always wins.
	req := Request{Task: "kafka transaction", Budget: 15}
	ms := []model.Memory{
		// lo-conf placed first (gets rank 1 in tied signals) — but confidence should overcome RRF rank advantage.
		{ID: "lo-conf", Kind: "decision", Content: "kafka transaction pattern", Authority: "user", TokenCost: 10, Confidence: 0.30},
		{ID: "hi-conf", Kind: "decision", Content: "kafka transaction pattern", Authority: "user", TokenCost: 10, Confidence: 0.95},
	}
	p := Plan(req, ms)
	if len(p.Selected) == 0 {
		t.Fatal("at least one should be selected")
	}
	// Only one fits (budget=15, each costs 10; 10+10>15). The winner must be hi-conf.
	if p.Selected[0].ID != "hi-conf" {
		t.Errorf("high-confidence item should win; got %s", p.Selected[0].ID)
	}
}

func TestConfidenceZeroTreatedAsOne(t *testing.T) {
	// Confidence=0 (unset) must not penalize the item — it should be treated as 1.0.
	p := Plan(Request{Task: "kafka", Budget: 100}, []model.Memory{
		{ID: "unset", Kind: "decision", Content: "kafka outbox", Authority: "user", TokenCost: 10, Confidence: 0.0},
	})
	if !hasID(p, "unset") {
		t.Error("Confidence=0 should be treated as 1.0, item should be selected")
	}
	for _, c := range p.Candidates {
		if c.ID == "unset" && math.Abs(c.Confidence-1.0) > 0.001 {
			t.Errorf("confWeight(0) should return 1.0, got %.4f", c.Confidence)
		}
	}
}

func TestAuthorityOrdering(t *testing.T) {
	// user (1.0) > commit (0.94) > doc (0.86); inference (0.55) is hard-rejected.
	req := Request{Task: "kafka transaction", Budget: 500}
	ms := []model.Memory{
		{ID: "inf", Kind: "decision", Content: "kafka transaction", Authority: "inference", TokenCost: 10},
		{ID: "doc", Kind: "decision", Content: "kafka transaction", Authority: "doc", TokenCost: 10},
		{ID: "commit", Kind: "decision", Content: "kafka transaction", Authority: "commit", TokenCost: 10},
		{ID: "user", Kind: "decision", Content: "kafka transaction", Authority: "user", TokenCost: 10},
	}
	p := Plan(req, ms)
	// inference must be excluded.
	if hasID(p, "inf") {
		t.Error("inference-authority item should be rejected (auth < 0.6)")
	}
	// user must rank first among eligible items.
	if len(p.Selected) == 0 || p.Selected[0].ID != "user" {
		t.Errorf("user-authority item should rank first; got %v", ids(p))
	}
}

func TestKindBoostDecisionBeforeFact(t *testing.T) {
	// Decision (kindBoost=1.0) vs fact (kindBoost=0.65) at equal authority.
	// Decision gets extra task-relevant content to ensure it wins both kindBoost AND BM25 signals.
	req := Request{Task: "kafka transaction outbox", Budget: 15}
	ms := []model.Memory{
		// fact is listed first (gets RRF rank 1 in ties) but has lower kindBoost and fewer task terms.
		{ID: "fact", Kind: "fact", Content: "kafka transaction", Authority: "user", TokenCost: 10},
		// decision has all 3 task terms: wins BM25/affinity/RRF, confirming kindBoost integration.
		{ID: "decision", Kind: "decision", Content: "kafka transaction outbox pattern", Authority: "user", TokenCost: 10},
	}
	p := Plan(req, ms)
	// Budget=15, each=10, only one fits.
	if len(p.Selected) != 1 {
		t.Fatalf("expected exactly 1 selected, got %v", ids(p))
	}
	if p.Selected[0].ID != "decision" {
		t.Errorf("decision (higher kindBoost + more task terms) should win; got %s", p.Selected[0].ID)
	}
}


// ─── output structure ────────────────────────────────────────────────────────

func TestStablePrefixAndVariableContextPartition(t *testing.T) {
	req := Request{Task: "kafka", Budget: 500}
	ms := []model.Memory{
		{ID: "dec", Kind: "decision", Content: "kafka decision", Authority: "user", TokenCost: 30},
		{ID: "con", Kind: "constraint", Content: "kafka constraint", Authority: "user", TokenCost: 30},
		{ID: "obs", Kind: "observation", Content: "kafka observation", Authority: "commit", TokenCost: 30},
		{ID: "sta", Kind: "state", Content: "kafka state", Authority: "commit", TokenCost: 30},
		{ID: "fac", Kind: "fact", Content: "kafka fact", Authority: "source", TokenCost: 30},
	}
	p := Plan(req, ms)
	// dec, con, fac → StablePrefix; obs, sta → VariableContext.
	stableIDs := map[string]bool{}
	for _, c := range p.StablePrefix {
		stableIDs[c.ID] = true
	}
	varIDs := map[string]bool{}
	for _, c := range p.VariableContext {
		varIDs[c.ID] = true
	}
	for _, mustBeStable := range []string{"dec", "con", "fac"} {
		if !stableIDs[mustBeStable] {
			t.Errorf("'%s' should be in StablePrefix, got stableIDs=%v", mustBeStable, stableIDs)
		}
	}
	for _, mustBeVar := range []string{"obs", "sta"} {
		if !varIDs[mustBeVar] {
			t.Errorf("'%s' should be in VariableContext, got varIDs=%v", mustBeVar, varIDs)
		}
	}
}

func TestStablePrefixContainsNoVariableKinds(t *testing.T) {
	req := Request{Task: "kafka state machine", Budget: 500}
	ms := []model.Memory{
		{ID: "state", Kind: "state", Content: "kafka state machine active", Authority: "user", TokenCost: 30},
		{ID: "dec", Kind: "decision", Content: "kafka state machine design", Authority: "user", TokenCost: 30},
	}
	p := Plan(req, ms)
	for _, c := range p.StablePrefix {
		if c.Kind == "state" || c.Kind == "observation" {
			t.Errorf("kind=%s should not appear in StablePrefix", c.Kind)
		}
	}
}

func TestSelectedTokensMatchesSum(t *testing.T) {
	req := Request{Task: "kafka", Budget: 200}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka outbox", Authority: "user", TokenCost: 50},
		{ID: "b", Kind: "constraint", Content: "kafka constraint", Authority: "source", TokenCost: 70},
		{ID: "c", Kind: "fact", Content: "kafka fact", Authority: "user", TokenCost: 30},
	}
	p := Plan(req, ms)
	sum := 0
	for _, c := range p.Selected {
		sum += c.Tokens
	}
	if p.SelectedTokens != sum {
		t.Errorf("SelectedTokens=%d does not match sum of selected.Tokens=%d", p.SelectedTokens, sum)
	}
}

// ─── budget utilization ──────────────────────────────────────────────────────

func TestFillPassMaximizesBudgetUtilization(t *testing.T) {
	// Primary greedy pick: A(70 tok). B(70 tok) doesn't fit. C(25 tok) fills remaining 30.
	req := Request{Task: "kafka transaction", Budget: 100}
	ms := []model.Memory{
		{ID: "a", Kind: "decision", Content: "kafka transaction outbox exactly once", Authority: "user", TokenCost: 70},
		{ID: "b", Kind: "decision", Content: "kafka transaction retry backoff strategy", Authority: "user", TokenCost: 70},
		{ID: "c", Kind: "fact", Content: "kafka transaction fact", Authority: "source", TokenCost: 25},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > req.Budget {
		t.Errorf("budget exceeded: %d > %d", p.SelectedTokens, req.Budget)
	}
	// C(25) must be selected to fill the remaining 30 tokens after A(70).
	if !hasID(p, "c") {
		t.Logf("selected: %v, tokens: %d", ids(p), p.SelectedTokens)
		t.Error("fill pass should have selected 'c' to fill remaining budget")
	}
}

func TestFillPassAfterSingletonRescue(t *testing.T) {
	// Singleton rescue replaces s1 with "big" (95 tokens, budget=100).
	// Remaining budget after rescue = 5. No fill items fit.
	// → Test ensures budget invariant holds post-rescue.
	req := Request{Task: "kafka transaction critical outbox", Budget: 100}
	ms := []model.Memory{
		{ID: "big", Kind: "decision", Content: "kafka transaction critical outbox exactly once delivery", Authority: "user", TokenCost: 95},
		{ID: "s1", Kind: "observation", Content: "network packet loss", Authority: "commit", TokenCost: 50},
		{ID: "tiny", Kind: "fact", Content: "kafka fact note", Authority: "source", TokenCost: 4},
	}
	p := Plan(req, ms)
	if p.SelectedTokens > req.Budget {
		t.Errorf("budget exceeded after rescue+fill: %d > %d", p.SelectedTokens, req.Budget)
	}
	// "tiny" (4 tok) should fill remaining budget (100-95=5 ≥ 4).
	if hasID(p, "big") && !hasID(p, "tiny") {
		t.Errorf("fill pass should add 'tiny' (4 tok) after singleton rescue; selected=%v tokens=%d", ids(p), p.SelectedTokens)
	}
}

// ─── graph centrality ────────────────────────────────────────────────────────

func TestGraphCentralityDetectedForCodeKind(t *testing.T) {
	req := Request{Task: "kafka handler", Budget: 100}
	ms := []model.Memory{
		{ID: "code", Kind: "code", Content: "handler/kafka.go implements the consumer", Authority: "source", TokenCost: 20},
	}
	p := Plan(req, ms)
	for _, c := range p.Candidates {
		if c.ID == "code" && c.Graph == 0 {
			t.Error("code kind should produce non-zero graph centrality score")
		}
	}
}

func TestGraphCentralityDetectedByPathSeparators(t *testing.T) {
	req := Request{Task: "kafka handler", Budget: 100}
	ms := []model.Memory{
		{ID: "pathref", Kind: "decision", Content: "See internal/kafka/handler.go for implementation", Authority: "user", TokenCost: 20},
	}
	p := Plan(req, ms)
	for _, c := range p.Candidates {
		if c.ID == "pathref" && c.Graph == 0 {
			t.Error("path separators in content should produce non-zero graph score")
		}
	}
}

// ─── score function unit tests ───────────────────────────────────────────────

func TestScoreHardStaleGetsDensityMinusInf(t *testing.T) {
	req := Request{Task: "kafka", Budget: 100}
	m := model.Memory{ID: "s", Kind: "decision", Content: "kafka", Authority: "user", InvalidatedAtRevision: "old"}
	c := Score(req, m)
	if !math.IsInf(c.Density, -1) {
		t.Errorf("hard-stale item must have Density=-Inf, got %.4f", c.Density)
	}
}

func TestScoreLowAuthorityGetsDensityMinusInf(t *testing.T) {
	req := Request{Task: "kafka", Budget: 100}
	m := model.Memory{ID: "s", Kind: "fact", Content: "kafka", Authority: "inference"}
	c := Score(req, m)
	if !math.IsInf(c.Density, -1) {
		t.Errorf("low-authority item must have Density=-Inf, got %.4f", c.Density)
	}
}

func TestScoreTokenCostEstimatedWhenZero(t *testing.T) {
	req := Request{Task: "kafka", Budget: 100}
	m := model.Memory{ID: "s", Kind: "decision", Content: "kafka transaction outbox", Authority: "user", TokenCost: 0}
	c := Score(req, m)
	if c.Tokens <= 0 {
		t.Errorf("zero TokenCost should be estimated, got %d", c.Tokens)
	}
}

func TestScoreSourceIDPrefixedNodeUsesRepositorySource(t *testing.T) {
	req := Request{Task: "kafka", Budget: 100}
	m := model.Memory{ID: "node:kafka/handler.go", Kind: "code", Content: "kafka handler", Authority: "source", TokenCost: 10}
	c := Score(req, m)
	if c.Source != "repository" {
		t.Errorf("node: prefix should set Source='repository', got %q", c.Source)
	}
}

func TestScoreEvidenceFromSourceField(t *testing.T) {
	req := Request{Task: "kafka", Budget: 100}
	m := model.Memory{ID: "e", Kind: "fact", Content: "kafka constraint", Authority: "doc", Source: "ADR-42", TokenCost: 10}
	c := Score(req, m)
	if c.Evidence != 1.0 {
		t.Errorf("non-empty Source should set Evidence=1.0, got %.4f", c.Evidence)
	}
}

func TestRRFScoreMonotonicallyDecreasesWithRank(t *testing.T) {
	s1 := rrfScore(1, 1, 1)
	s2 := rrfScore(2, 2, 2)
	s10 := rrfScore(10, 10, 10)
	if !(s1 > s2 && s2 > s10) {
		t.Errorf("rrfScore should decrease as rank increases: s1=%.5f s2=%.5f s10=%.5f", s1, s2, s10)
	}
}

func TestRRFScoreSymmetric(t *testing.T) {
	// Score should not depend on which signal is which.
	if rrfScore(1, 2, 3) != rrfScore(3, 1, 2) {
		t.Error("rrfScore should be commutative across signal ranks")
	}
}

// ─── table-driven comprehensive scenarios ────────────────────────────────────

func TestPlanTableDriven(t *testing.T) {
	type tc struct {
		name        string
		req         Request
		ms          []model.Memory
		wantMinSel  int
		wantMaxSel  int
		wantBudget  int // expected Plan.Budget; 0 = don't check
		mustHaveIDs []string
		mustNotHave []string
	}

	tests := []tc{
		{
			name:       "empty memories",
			req:        Request{Task: "kafka", Budget: 100},
			ms:         nil,
			wantMinSel: 0, wantMaxSel: 0,
		},
		{
			name:       "zero budget defaults to 4000",
			req:        Request{Task: "kafka", Budget: 0},
			ms:         []model.Memory{{ID: "a", Kind: "decision", Content: "kafka", Authority: "user", TokenCost: 10}},
			wantBudget: 4000,
			wantMinSel: 1,
		},
		{
			name: "all inference → 0 selected",
			req:  Request{Task: "kafka", Budget: 500},
			ms: []model.Memory{
				{ID: "x", Kind: "fact", Content: "kafka detail", Authority: "inference", TokenCost: 10},
				{ID: "y", Kind: "fact", Content: "kafka fact", Authority: "inference", TokenCost: 10},
			},
			wantMinSel:  0,
			wantMaxSel:  0,
			mustNotHave: []string{"x", "y"},
		},
		{
			name: "mixed authority: user+commit selected, inference rejected",
			req:  Request{Task: "kafka transaction", Budget: 500},
			ms: []model.Memory{
				{ID: "u", Kind: "decision", Content: "kafka transaction user decision", Authority: "user", TokenCost: 20},
				{ID: "c", Kind: "decision", Content: "kafka transaction commit", Authority: "commit", TokenCost: 20},
				{ID: "i", Kind: "fact", Content: "kafka transaction inference", Authority: "inference", TokenCost: 20},
			},
			mustHaveIDs: []string{"u", "c"},
			mustNotHave: []string{"i"},
		},
		{
			name: "failure kind gets higher kindBoost than fact",
			req:  Request{Task: "kafka", Budget: 15},
			ms: []model.Memory{
				{ID: "fail", Kind: "failure", Content: "kafka failure root cause", Authority: "test", TokenCost: 10},
				{ID: "fact", Kind: "fact", Content: "kafka general fact note", Authority: "test", TokenCost: 10},
			},
			mustHaveIDs: []string{"fail"},
		},
		{
			name: "two identical items: greedy picks both when budget allows",
			req:  Request{Task: "kafka", Budget: 100},
			ms: []model.Memory{
				{ID: "a", Kind: "decision", Content: "kafka outbox pattern", Authority: "user", TokenCost: 30},
				{ID: "b", Kind: "decision", Content: "kafka outbox pattern", Authority: "user", TokenCost: 30},
			},
			wantMinSel:  2,
			mustHaveIDs: []string{"a", "b"},
		},
		{
			name: "item exactly at budget is selected",
			req:  Request{Task: "kafka", Budget: 50},
			ms: []model.Memory{
				{ID: "exact", Kind: "decision", Content: "kafka transaction outbox", Authority: "user", TokenCost: 50},
			},
			mustHaveIDs: []string{"exact"},
		},
		{
			name: "item one token over budget is not selected",
			req:  Request{Task: "kafka", Budget: 49},
			ms: []model.Memory{
				{ID: "over", Kind: "decision", Content: "kafka transaction outbox", Authority: "user", TokenCost: 50},
			},
			wantMinSel:  0,
			wantMaxSel:  0,
			mustNotHave: []string{"over"},
		},
		{
			name: "source authority outranks doc authority",
			req:  Request{Task: "kafka transaction", Budget: 15},
			ms: []model.Memory{
				{ID: "doc", Kind: "decision", Content: "kafka transaction pattern", Authority: "doc", TokenCost: 10},
				{ID: "src", Kind: "decision", Content: "kafka transaction pattern", Authority: "source", TokenCost: 10},
			},
			mustHaveIDs: []string{"src"},
		},
		{
			name: "constraint kind in stable prefix",
			req:  Request{Task: "kafka", Budget: 200},
			ms: []model.Memory{
				{ID: "con", Kind: "constraint", Content: "kafka constraint", Authority: "user", TokenCost: 30},
				{ID: "obs", Kind: "observation", Content: "kafka observation", Authority: "commit", TokenCost: 30},
			},
			mustHaveIDs: []string{"con", "obs"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := Plan(tc.req, tc.ms)

			if tc.wantBudget != 0 && p.Budget != tc.wantBudget {
				t.Errorf("Budget: want %d, got %d", tc.wantBudget, p.Budget)
			}
			if tc.wantMinSel > 0 && len(p.Selected) < tc.wantMinSel {
				t.Errorf("selected count: want ≥%d, got %d (%v)", tc.wantMinSel, len(p.Selected), ids(p))
			}
			if tc.wantMaxSel > 0 && len(p.Selected) > tc.wantMaxSel {
				t.Errorf("selected count: want ≤%d, got %d (%v)", tc.wantMaxSel, len(p.Selected), ids(p))
			}
			if tc.wantMinSel == 0 && tc.wantMaxSel == 0 && tc.ms != nil && len(p.Selected) != 0 {
				// both 0 with non-nil ms means we explicitly want 0
				if len(tc.mustNotHave) == 0 && len(tc.mustHaveIDs) == 0 {
					t.Errorf("expected 0 selected, got %v", ids(p))
				}
			}
			for _, id := range tc.mustHaveIDs {
				if !hasID(p, id) {
					t.Errorf("must-have ID %q missing from selection %v", id, ids(p))
				}
			}
			for _, id := range tc.mustNotHave {
				if hasID(p, id) {
					t.Errorf("must-not-have ID %q appears in selection %v", id, ids(p))
				}
			}
			// Budget invariant: never exceeded.
			if p.SelectedTokens > p.Budget {
				t.Errorf("budget invariant violated: selected=%d > budget=%d", p.SelectedTokens, p.Budget)
			}
			// StablePrefix + VariableContext must equal Selected.
			totalPartitioned := len(p.StablePrefix) + len(p.VariableContext)
			if totalPartitioned != len(p.Selected) {
				t.Errorf("partition mismatch: StablePrefix(%d) + VariableContext(%d) ≠ Selected(%d)",
					len(p.StablePrefix), len(p.VariableContext), len(p.Selected))
			}
			// SelectedTokens must match sum of Selected.Tokens.
			sum := 0
			for _, c := range p.Selected {
				sum += c.Tokens
			}
			if p.SelectedTokens != sum {
				t.Errorf("SelectedTokens=%d ≠ sum(selected.Tokens)=%d", p.SelectedTokens, sum)
			}
		})
	}
}
