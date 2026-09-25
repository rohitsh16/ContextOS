package retrieval

// RetrievalStage represents the stage or channel of retrieval that produced or scored a candidate.
type RetrievalStage string

const (
	StageExactPath  RetrievalStage = "exact_path"
	StageBasename   RetrievalStage = "basename"
	StageLexical    RetrievalStage = "lexical"
	StageSemantic   RetrievalStage = "semantic"
	StageSymbol     RetrievalStage = "symbol"
	StageEntity     RetrievalStage = "entity"
	StageGraph      RetrievalStage = "graph"
	StageExpansion  RetrievalStage = "expansion"
	StageRerank     RetrievalStage = "rerank"
)

// CandidateTrace records per-candidate provenance, channel scores, and ranking diagnostics.
type CandidateTrace struct {
	ID              string           `json:"id"`
	Path            string           `json:"path"`
	Name            string           `json:"name"`
	Stages          []RetrievalStage `json:"stages"`
	LexicalScore    float64          `json:"lexical_score"`
	SemanticScore   float64          `json:"semantic_score"`
	SymbolScore     float64          `json:"symbol_score"`
	EntityScore     float64          `json:"entity_score"`
	PathScore       float64          `json:"path_score"`
	GraphScore      float64          `json:"graph_score"`
	FinalScore      float64          `json:"final_score"`
	Rank            int              `json:"rank"`
	Admissible      bool             `json:"admissible"`
	RejectionReason string           `json:"rejection_reason,omitempty"`
}

// HasStage returns true if the candidate passed through the given retrieval stage.
func (ct *CandidateTrace) HasStage(stage RetrievalStage) bool {
	for _, s := range ct.Stages {
		if s == stage {
			return true
		}
	}
	return false
}

// AddStage appends a stage to the candidate trace if not already present.
func (ct *CandidateTrace) AddStage(stage RetrievalStage) {
	if !ct.HasStage(stage) {
		ct.Stages = append(ct.Stages, stage)
	}
}

// QueryRetrievalTrace records the comprehensive trace of an individual query retrieval (R18.1 §6 & §38).
type QueryRetrievalTrace struct {
	QueryID                   string           `json:"query_id"`
	Query                     string           `json:"query"`
	CandidateCount            int              `json:"candidate_count"`
	CandidateGenerationRecall float64          `json:"candidate_generation_recall"`
	TargetPresent             bool             `json:"target_present"`
	TargetRank                int              `json:"target_rank"`
	TargetScore               float64          `json:"target_score"`
	TopCandidates             []CandidateTrace `json:"top_candidates"`
	RetrievalStages           []RetrievalStage `json:"retrieval_stages"`
	AdmissionRejections       []string         `json:"admission_rejections"`
	ExpansionNodes            []string         `json:"expansion_nodes"`
	FinalRankedResults        []CandidateTrace `json:"final_ranked_results"`
	FailureDiagnosis          string           `json:"failure_diagnosis,omitempty"`
}

// DiagnoseFailure determines whether a failed query was due to candidate generation or ranking.
func (t *QueryRetrievalTrace) DiagnoseFailure(targetPath string) string {
	if t.TargetPresent {
		if t.TargetRank > 10 {
			t.FailureDiagnosis = "ranking_failure: target was generated in candidate pool but ranked too low"
			return t.FailureDiagnosis
		}
		t.FailureDiagnosis = "success"
		return t.FailureDiagnosis
	}
	t.FailureDiagnosis = "candidate_generation_failure: target was completely absent from candidate pool"
	return t.FailureDiagnosis
}
