package compute

import (
	"regexp"
	"strings"

	"contextos/internal/textutil"
)

// TaskFeatures captures structural and semantic indicators of task complexity and uncertainty.
type TaskFeatures struct {
	QueryTokens          int     `json:"query_tokens"`
	FilesMentioned       int     `json:"files_mentioned"`
	SymbolsMentioned     int     `json:"symbols_mentioned"`
	DependencyDepth      int     `json:"dependency_depth"`
	ScopeSize            int     `json:"scope_size"`
	Ambiguity            float64 `json:"ambiguity"` // 0.0 (crystal clear) to 1.0 (vague)
	Novelty              float64 `json:"novelty"`   // 0.0 (seen repeatedly) to 1.0 (completely new)
	Risk                 float64 `json:"risk"`      // 0.0 (safe read-only) to 1.0 (production migration/security)
	ExpectedToolCalls    int     `json:"expected_tool_calls"`
	HistoricalDifficulty float64 `json:"historical_difficulty"` // 0.0 to 1.0
}

// TaskProfile contains the extracted features, composite difficulty score, and classified task tier.
type TaskProfile struct {
	Features   TaskFeatures `json:"features"`
	Difficulty float64      `json:"difficulty"` // Normalized 0.0 to 1.0
	Class      TaskClass    `json:"class"`
	CanBypass  bool         `json:"can_bypass"`       // True if task can execute deterministically without LLM
	Family     string       `json:"family,omitempty"` // preregistered benchmark family
}

var (
	filePathRegex = regexp.MustCompile(`\b[\w-]+\.(go|ts|js|py|rs|c|cpp|h|md|json|yaml|yml|sql)\b`)
	symbolRegex   = regexp.MustCompile(`\b(func|type|class|interface|struct)\s+([A-Za-z0-9_]+)\b`)
)

// TaskProfiler evaluates user queries and context signals to compute difficulty and task class.
type TaskProfiler struct {
	// Feature weighting configuration
	wScope      float64
	wDependency float64
	wAmbiguity  float64
	wNovelty    float64
	wRisk       float64
	wHistory    float64
}

// NewTaskProfiler creates a deterministic task profiler with calibrated weights.
func NewTaskProfiler() *TaskProfiler {
	return &TaskProfiler{
		wScope:      0.20,
		wDependency: 0.15,
		wAmbiguity:  0.20,
		wNovelty:    0.15,
		wRisk:       0.20,
		wHistory:    0.10,
	}
}

// Profile analyzes a task string, query tokens, and contextual repository metrics.
func (p *TaskProfiler) Profile(task string, historicalDifficulty float64) TaskProfile {
	tokens := textutil.EstimateTokens(task)
	clean := strings.ToLower(task)

	files := len(filePathRegex.FindAllString(task, -1))
	symbols := len(symbolRegex.FindAllString(task, -1))

	// Ambiguity estimation: short questions with question marks, 'why', 'how', or fuzzy words
	ambiguity := 0.2
	if tokens < 10 {
		ambiguity += 0.3
	}
	if strings.Contains(clean, "how") || strings.Contains(clean, "why") || strings.Contains(clean, "investigate") {
		ambiguity += 0.3
	}
	if strings.Contains(clean, "maybe") || strings.Contains(clean, "perhaps") || strings.Contains(clean, "check") {
		ambiguity += 0.2
	}
	if ambiguity > 1.0 {
		ambiguity = 1.0
	}

	// Risk estimation: keywords related to delete, drop, migration, security, concurrency, race
	risk := 0.1
	if strings.Contains(clean, "auth") || strings.Contains(clean, "security") || strings.Contains(clean, "permission") {
		risk += 0.4
	}
	if strings.Contains(clean, "race") || strings.Contains(clean, "deadlock") || strings.Contains(clean, "concurrency") {
		risk += 0.4
	}
	if strings.Contains(clean, "delete") || strings.Contains(clean, "drop") || strings.Contains(clean, "migrate") {
		risk += 0.3
	}
	if risk > 1.0 {
		risk = 1.0
	}

	// Scope size: based on files, symbols, tokens and architectural keywords
	scopeSize := files*5 + symbols*3 + tokens/50
	if strings.Contains(clean, "refactor") || strings.Contains(clean, "redesign") || strings.Contains(clean, "architecture") {
		scopeSize += 15
	}
	if strings.Contains(clean, "distributed") || strings.Contains(clean, "consensus") || strings.Contains(clean, "engine") {
		scopeSize += 15
	}
	if scopeSize < 1 {
		scopeSize = 1
	}

	normScope := float64(scopeSize) / 50.0
	if normScope > 1.0 {
		normScope = 1.0
	}

	depDepth := 1
	if files > 2 || symbols > 2 {
		depDepth = 3
	}
	if strings.Contains(clean, "trace") || strings.Contains(clean, "callers") || strings.Contains(clean, "graph") ||
		strings.Contains(clean, "distributed") || strings.Contains(clean, "consensus") {
		depDepth = 4
	}
	normDep := float64(depDepth) / 5.0

	novelty := 0.3
	if historicalDifficulty > 0 {
		novelty = 0.5 * (1.0 - historicalDifficulty)
	}

	// Check deterministic bypass: exact symbol, caller, import, or AST lookups
	canBypass := false
	if (strings.HasPrefix(clean, "where is") ||
		strings.HasPrefix(clean, "find definition") ||
		strings.HasPrefix(clean, "find symbol") ||
		strings.HasPrefix(clean, "find callers") ||
		strings.HasPrefix(clean, "find implementations") ||
		strings.HasPrefix(clean, "find where") ||
		strings.HasPrefix(clean, "find block size") ||
		strings.HasPrefix(clean, "locate") ||
		strings.HasPrefix(clean, "list imports") ||
		strings.HasPrefix(clean, "get method") ||
		strings.HasPrefix(clean, "callers of")) && files <= 2 {
		canBypass = true
	}

	// Difficulty: D = w1*Scope + w2*Dep + w3*Ambiguity + w4*Novelty + w5*Risk + w6*History
	difficulty := (p.wScope * normScope) +
		(p.wDependency * normDep) +
		(p.wAmbiguity * ambiguity) +
		(p.wNovelty * novelty) +
		(p.wRisk * risk) +
		(p.wHistory * historicalDifficulty)

	if difficulty < 0 {
		difficulty = 0
	}
	if difficulty > 1.0 {
		difficulty = 1.0
	}

	var class TaskClass
	if canBypass {
		class = T0Deterministic
	} else if difficulty < 0.25 {
		class = T1Trivial
	} else if difficulty < 0.50 {
		class = T2Moderate
	} else if difficulty < 0.75 {
		class = T3Difficult
	} else {
		class = T4Critical
	}

	expectedToolCalls := 1
	switch class {
	case T0Deterministic:
		expectedToolCalls = 0
	case T1Trivial:
		expectedToolCalls = 1
	case T2Moderate:
		expectedToolCalls = 3
	case T3Difficult:
		expectedToolCalls = 6
	case T4Critical:
		expectedToolCalls = 12
	}

	return TaskProfile{
		Features: TaskFeatures{
			QueryTokens:          tokens,
			FilesMentioned:       files,
			SymbolsMentioned:     symbols,
			DependencyDepth:      depDepth,
			ScopeSize:            scopeSize,
			Ambiguity:            ambiguity,
			Novelty:              novelty,
			Risk:                 risk,
			ExpectedToolCalls:    expectedToolCalls,
			HistoricalDifficulty: historicalDifficulty,
		},
		Difficulty: difficulty,
		Class:      class,
		CanBypass:  canBypass,
	}
}
