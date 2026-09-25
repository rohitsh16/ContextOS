package verification

// FailureCode identifies the diagnostic failure mode according to R17.5/R18 taxonomy (R17.5 §1.1).
type FailureCode string

const (
	CodeR0QueryParsing       FailureCode = "R0" // Query intent/entities not correctly understood
	CodeR1IndexIncomplete    FailureCode = "R1" // Required source never entered the index
	CodeR2IndexContamination FailureCode = "R2" // Ineligible/noisy source entered the index
	CodeR3RetrievalRecall    FailureCode = "R3" // Valid evidence exists but was not retrieved
	CodeR4Insufficiency      FailureCode = "R4" // Retrieved context does not contain enough evidence
	CodeR5ContextIntegrity   FailureCode = "R5" // Selected evidence is stale/conflicting/invalid
	CodeR6ReasoningFailure   FailureCode = "R6" // Model had sufficient evidence but reasoned incorrectly
	CodeR7ClaimSupport       FailureCode = "R7" // Answer contains unsupported claims
	CodeR8VerifierFailure    FailureCode = "R8" // Verifier incorrectly accepted or rejected
	CodeR9AbstentionFailure  FailureCode = "R9" // System answered despite insufficient evidence
)

// CorrectnessTrace is the primary empirical artifact recorded for every query (R18.2 §47).
type CorrectnessTrace struct {
	QueryID            string        `json:"query_id"`
	Revision           string        `json:"revision"`
	EvidenceConsidered int           `json:"evidence_considered"`
	EvidenceRejected   int           `json:"evidence_rejected"`
	EvidenceSelected   int           `json:"evidence_selected"`
	PollutionCount     int           `json:"pollution_count"`
	MissingEvidence    []string      `json:"missing_evidence,omitempty"`
	SufficiencyScore   float64       `json:"sufficiency_score"`
	SufficiencyStatus  string        `json:"sufficiency_status"`
	Claims             []AtomicClaim `json:"claims,omitempty"`
	AnswerStatus       AnswerStatus  `json:"answer_status"`
	Action             GateAction    `json:"action"`
	FailureCode        FailureCode   `json:"failure_code,omitempty"`
	Tokens             int           `json:"tokens"`
	CostUSD            float64       `json:"cost_usd"`
	LatencyMS          int64         `json:"latency_ms"`
}
