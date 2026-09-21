package state

import "time"

// Constraint defines an invariant or operational boundary that must not be violated.
type Constraint struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Source      string `json:"source,omitempty"` // "user", "repo", "architecture", "security"
}

// Fact defines an established ground truth observed in code or system state.
type Fact struct {
	ID        string    `json:"id"`
	Content   string    `json:"content"`
	Location  string    `json:"location,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Decision records an intentional technical commitment and its rationale.
type Decision struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Rationale string    `json:"rationale"`
	Timestamp time.Time `json:"timestamp"`
	Author    string    `json:"author,omitempty"`
}

// Hypothesis represents an unverified theory or implementation path under consideration.
type Hypothesis struct {
	ID        string    `json:"id"`
	Claim     string    `json:"claim"`
	Likelihood float64  `json:"likelihood"`
	Reason    string    `json:"reason,omitempty"`
}

// Question represents an unresolved design or factual query.
type Question struct {
	ID       string `json:"id"`
	Question string `json:"question"`
	Answer   string `json:"answer,omitempty"`
	Resolved bool   `json:"resolved"`
}

// ActionItem represents a planned or executed operational step.
type ActionItem struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
}

// DecisionState is the durable, non-ephemeral representation of an engineering task.
// This replaces re-injecting 100+ raw transcript turns into LLM context.
type DecisionState struct {
	Objective          string       `json:"objective"`
	Constraints        []Constraint `json:"constraints"`
	Facts              []Fact       `json:"facts"`
	Evidence           []Evidence   `json:"evidence"`
	Decisions          []Decision   `json:"decisions"`
	Hypotheses         []Hypothesis `json:"hypotheses"`
	RejectedHypotheses []Hypothesis `json:"rejected_hypotheses"`
	Uncertainties      []Uncertainty `json:"uncertainties"`
	OpenQuestions      []Question   `json:"open_questions"`
	NextActions        []ActionItem `json:"next_actions"`
	Confidence         float64      `json:"confidence"`
	Risk               float64      `json:"risk"`
	CreatedAt          time.Time    `json:"created_at"`
	UpdatedAt          time.Time    `json:"updated_at"`
}

// NewDecisionState initializes an empty decision state for a given objective.
func NewDecisionState(objective string) DecisionState {
	now := time.Now().UTC()
	return DecisionState{
		Objective:          objective,
		Constraints:        make([]Constraint, 0),
		Facts:              make([]Fact, 0),
		Evidence:           make([]Evidence, 0),
		Decisions:          make([]Decision, 0),
		Hypotheses:         make([]Hypothesis, 0),
		RejectedHypotheses: make([]Hypothesis, 0),
		Uncertainties:      make([]Uncertainty, 0),
		OpenQuestions:      make([]Question, 0),
		NextActions:        make([]ActionItem, 0),
		Confidence:         0.50,
		Risk:               0.50,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

// AddFact records a new validated fact, deduplicating identical content.
func (ds *DecisionState) AddFact(content, location string) {
	for _, f := range ds.Facts {
		if f.Content == content {
			return
		}
	}
	ds.Facts = append(ds.Facts, Fact{
		ID:        location + ":" + content,
		Content:   content,
		Location:  location,
		Timestamp: time.Now().UTC(),
	})
	ds.UpdatedAt = time.Now().UTC()
}

// RecordDecision commits an architectural or code decision, deduplicating identical title.
func (ds *DecisionState) RecordDecision(title, rationale, author string) {
	for _, d := range ds.Decisions {
		if d.Title == title {
			return
		}
	}
	ds.Decisions = append(ds.Decisions, Decision{
		ID:        title,
		Title:     title,
		Rationale: rationale,
		Timestamp: time.Now().UTC(),
		Author:    author,
	})
	ds.UpdatedAt = time.Now().UTC()
}

// RejectHypothesis moves a hypothesis to the rejected list with rationale to prevent re-exploration.
func (ds *DecisionState) RejectHypothesis(hyp Hypothesis, reason string) {
	hyp.Reason = reason
	// Remove from active hypotheses
	var active []Hypothesis
	for _, h := range ds.Hypotheses {
		if h.ID != hyp.ID {
			active = append(active, h)
		}
	}
	ds.Hypotheses = active
	ds.RejectedHypotheses = append(ds.RejectedHypotheses, hyp)
	ds.UpdatedAt = time.Now().UTC()
}
