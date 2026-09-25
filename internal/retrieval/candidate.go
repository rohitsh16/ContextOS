package retrieval

// Candidate represents an indexed code or memory entity scored and ranked during retrieval.
type Candidate struct {
	ID        string `json:"id"`
	NodeID    string `json:"node_id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Path      string `json:"path"`
	Package   string `json:"package,omitempty"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	Signature string `json:"signature"`
	Content   string `json:"content"`
	Tokens    int    `json:"tokens,omitempty"`

	// Multi-stage score components
	LexicalScore  float64 `json:"lexical_score"`
	GraphScore    float64 `json:"graph_score"`
	SemanticScore float64 `json:"semantic_score"`
	EntityScore   float64 `json:"entity_score,omitempty"`
	PathScore     float64 `json:"path_score,omitempty"`
	Score         float64 `json:"score"`

	// Stage indicates which pipeline stage generated or retained this candidate
	// (e.g. "exact", "trigram", "fts", "graph_expanded", "reranked")
	Stage string `json:"stage"`

	// Trace carries the granular multi-channel provenance and diagnostic scores
	Trace *CandidateTrace `json:"trace,omitempty"`

	// Provenance tracks how the candidate was matched (e.g. symbol name, path, reference)
	Provenance []string `json:"provenance,omitempty"`
}
