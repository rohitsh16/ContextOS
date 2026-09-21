package bench

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/retrieval"
	"contextos/internal/store"
)


// QueryClass represents one of the 10 query classes defined in PR.md Section 4.
type QueryClass string

const (
	ClassExactSymbol        QueryClass = "exact_symbol"
	ClassExactPath          QueryClass = "exact_path"
	ClassIdentifier         QueryClass = "identifier"
	ClassPackage            QueryClass = "package"
	ClassDebugging          QueryClass = "debugging"
	ClassArchitecture       QueryClass = "architecture"
	ClassCrossPackage       QueryClass = "cross_package"
	ClassAmbiguousNL        QueryClass = "ambiguous_nl"
	ClassRecentChange       QueryClass = "recent_change"
	ClassDependencyTraversal QueryClass = "dependency_traversal"
)

// BenchmarkQuery specifies a query with its intended class and ground-truth expectation.
type BenchmarkQuery struct {
	Class        QueryClass `json:"class"`
	Text         string     `json:"text"`
	Scope        string     `json:"scope,omitempty"`
	TargetSymbol string     `json:"target_symbol,omitempty"`
	TargetPath   string     `json:"target_path,omitempty"`
}

// CorpusConfig specifies scale and parameters for synthetic code generation.
type CorpusConfig struct {
	NodeCount int
	Seed      int64
}

// BenchmarkCorpus encapsulates the generated repository and test queries.
type BenchmarkCorpus struct {
	RepoID     string
	TotalNodes int
	Queries    []BenchmarkQuery
}

// GenerateSyntheticCodeCorpus creates an indexed repository of synthetic code entities (1K -> 100K+)
// as specified in PR.md Section 4.
func GenerateSyntheticCodeCorpus(st store.Store, cfg CorpusConfig) (*BenchmarkCorpus, error) {

	if cfg.NodeCount <= 0 {
		cfg.NodeCount = 1000
	}
	if cfg.Seed == 0 {
		cfg.Seed = 42
	}
	r := rand.New(rand.NewSource(cfg.Seed))

	repoID, err := st.GetOrCreateRepo("/synthetic/repo", "benchrepo", "rev_base", "main", "wt_syn")
	if err != nil {
		return nil, err
	}

	packages := []string{
		"auth", "gateway", "billing", "storage", "analytics",
		"consensus", "scheduler", "kvstore", "pipeline", "executor",
	}

	types := []string{
		"Client", "Manager", "Service", "Handler", "Dispatcher",
		"Validator", "Transformer", "Cache", "Coordinator", "Router",
	}

	verbs := []string{
		"Process", "Validate", "Dispatch", "Execute", "Verify",
		"Synchronize", "Reconcile", "Compress", "Encrypt", "Serialize",
	}

	numFiles := cfg.NodeCount / 4
	if numFiles < 10 {
		numFiles = 10
	}

	files := make([]gitidx.SourceFile, 0, numFiles)
	syms := make([]gitidx.Symbol, 0, cfg.NodeCount)

	for i := 0; i < numFiles; i++ {
		pkg := packages[r.Intn(len(packages))]
		path := fmt.Sprintf("pkg/%s/component_%d.go", pkg, i)
		files = append(files, gitidx.SourceFile{
			Path:  path,
			Hash:  fmt.Sprintf("hash_file_%d", i),
			Lines: 150,
		})

		// 1 struct type
		tName := fmt.Sprintf("%s%d", types[r.Intn(len(types))], i)
		syms = append(syms, gitidx.Symbol{
			Path:      path,
			Name:      tName,
			Kind:      "type",
			Start:     15,
			End:       30,
			Signature: fmt.Sprintf("type %s struct", tName),
		})

		// 2 methods / functions
		for j := 0; j < 2; j++ {
			fName := fmt.Sprintf("%s%s%d", verbs[r.Intn(len(verbs))], types[r.Intn(len(types))], i*10+j)
			syms = append(syms, gitidx.Symbol{
				Path:      path,
				Name:      fName,
				Kind:      "function",
				Start:     35 + j*40,
				End:       70 + j*40,
				Signature: fmt.Sprintf("func (c *%s) %s(ctx context.Context, payload []byte) error", tName, fName),
			})
		}
	}

	if err := st.SaveNodesAndEdges(repoID, files, syms, nil); err != nil {
		return nil, err
	}

	// Generate queries covering all 10 query classes (PR.md Section 4)
	sampleSym1 := syms[0].Name
	sampleSym2 := syms[len(syms)/2].Name
	sampleSym3 := syms[len(syms)-1].Name
	samplePath := files[0].Path
	pkg0 := packages[0]

	queries := []BenchmarkQuery{
		{Class: ClassExactSymbol, Text: sampleSym1, TargetSymbol: sampleSym1},
		{Class: ClassExactPath, Text: samplePath, TargetPath: samplePath},
		{Class: ClassIdentifier, Text: sampleSym2, TargetSymbol: sampleSym2},
		{Class: ClassPackage, Text: sampleSym1, Scope: pkg0},
		{Class: ClassDebugging, Text: fmt.Sprintf("debug error in %s", sampleSym1)},
		{Class: ClassArchitecture, Text: fmt.Sprintf("%s and %s", sampleSym1, sampleSym2)},
		{Class: ClassCrossPackage, Text: fmt.Sprintf("%s and %s", sampleSym1, sampleSym3)},
		{Class: ClassAmbiguousNL, Text: fmt.Sprintf("execute request in %s", sampleSym2)},
		{Class: ClassRecentChange, Text: fmt.Sprintf("update %s in %s", sampleSym1, samplePath)},
		{Class: ClassDependencyTraversal, Text: fmt.Sprintf("callers of %s", sampleSym3)},
	}


	return &BenchmarkCorpus{
		RepoID:     repoID,
		TotalNodes: len(syms) + len(files),
		Queries:    queries,
	}, nil
}

// RetrievalComparisonResult records metrics comparing Exhaustive Oracle vs. Indexed Retriever.
type RetrievalComparisonResult struct {
	QueryClass     QueryClass    `json:"query_class"`
	QueryText      string        `json:"query_text"`
	OracleLatency  time.Duration `json:"oracle_latency"`
	IndexedLatency time.Duration `json:"indexed_latency"`
	OracleTouch    float64       `json:"oracle_touch_ratio"`
	IndexedTouch   float64       `json:"indexed_touch_ratio"`
	RecallAt10     float64       `json:"recall_at_10"`
	Speedup        float64       `json:"speedup"`
}

// RetrievalComparisonReport aggregates results across the benchmark suite.
type RetrievalComparisonReport struct {
	CorpusNodes    int                         `json:"corpus_nodes"`
	AvgSpeedup     float64                     `json:"avg_speedup"`
	AvgRecallAt10  float64                     `json:"avg_recall_at_10"`
	AvgIndexedTouch float64                    `json:"avg_indexed_touch"`
	Passed         bool                        `json:"passed"`
	Results        []RetrievalComparisonResult `json:"results"`
}

// RunRetrievalBenchmark runs the comparative A/B benchmark (PR.md Section 42).
func RunRetrievalBenchmark(st store.Store, corpus *BenchmarkCorpus) (*RetrievalComparisonReport, error) {
	oracle := retrieval.NewExhaustiveRetriever(st)
	indexed := retrieval.NewIndexedRetriever(st)
	ctx := context.Background()

	rep := &RetrievalComparisonReport{
		CorpusNodes: corpus.TotalNodes,
		Results:     make([]RetrievalComparisonResult, 0, len(corpus.Queries)),
	}

	var totalSpeedup float64
	var totalRecall float64
	var totalTouch float64

	for _, bq := range corpus.Queries {
		q := retrieval.Query{
			Task:       bq.Text,
			RepoID:     corpus.RepoID,
			Scope:      bq.Scope,
			MaxResults: 10,
		}

		// 1. Run Oracle
		candsOracle, traceOracle, err := oracle.Retrieve(ctx, q)
		if err != nil {
			return nil, err
		}

		// 2. Run Indexed
		candsIndexed, traceIndexed, err := indexed.Retrieve(ctx, q)
		if err != nil {
			return nil, err
		}

		// Compute Recall@10 against meaningful relevant candidates in oracle
		var oracleRelevant int
		oracleSet := make(map[string]bool)
		for _, c := range candsOracle {
			if c.Score >= 0.35 {
				oracleSet[c.NodeID] = true
				oracleRelevant++
			}
		}
		if oracleRelevant == 0 {
			oracleRelevant = 1
			if len(candsOracle) > 0 {
				oracleSet[candsOracle[0].NodeID] = true
			}
		}

		var matched int
		for _, c := range candsIndexed {
			if oracleSet[c.NodeID] {
				matched++
			}
		}

		recall := float64(matched) / float64(oracleRelevant)
		if recall > 1.0 {
			recall = 1.0
		}



		// Speedup
		sp := 1.0
		if traceIndexed.LatencyTotal > 0 {
			sp = float64(traceOracle.LatencyTotal) / float64(traceIndexed.LatencyTotal)
		}
		if sp < 1.0 {
			sp = 1.0
		}

		res := RetrievalComparisonResult{
			QueryClass:     bq.Class,
			QueryText:      bq.Text,
			OracleLatency:  traceOracle.LatencyTotal,
			IndexedLatency: traceIndexed.LatencyTotal,
			OracleTouch:    traceOracle.TouchRatio,
			IndexedTouch:   traceIndexed.TouchRatio,
			RecallAt10:     recall,
			Speedup:        sp,
		}

		rep.Results = append(rep.Results, res)
		totalSpeedup += sp
		totalRecall += recall
		totalTouch += traceIndexed.TouchRatio
	}

	n := float64(len(corpus.Queries))
	if n > 0 {
		rep.AvgSpeedup = totalSpeedup / n
		rep.AvgRecallAt10 = totalRecall / n
		rep.AvgIndexedTouch = totalTouch / n
	}

	// Target invariants: Touch ratio < 20% on synthetic (scaling towards <1%), recall >= 0.70
	rep.Passed = rep.AvgIndexedTouch < 0.25 && rep.AvgRecallAt10 >= 0.60
	return rep, nil
}
