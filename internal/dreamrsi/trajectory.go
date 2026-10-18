package dreamrsi

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"contextos/internal/evaluation"
	"contextos/internal/model"
)

// TrajectoryNode represents a discrete state or action step in an agent's exploration search tree.
type TrajectoryNode struct {
	ID               string    `json:"id"`
	ParentID         string    `json:"parent_id,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
	SessionID        string    `json:"session_id"`
	Task             string    `json:"task"`
	RepoRevision     string    `json:"repo_revision,omitempty"`
	ActionTaken      string    `json:"action_taken"`
	ToolName         string    `json:"tool_name,omitempty"`
	Prompt           string    `json:"prompt,omitempty"`
	Output           string    `json:"output,omitempty"`
	IsFailure        bool      `json:"is_failure"`
	IsBacktrack      bool      `json:"is_backtrack"`
	BranchDepth      int       `json:"branch_depth"`
	StepTokenCost    int       `json:"step_token_cost"`
	ContextTokens    int       `json:"context_tokens"`
	CandidatesScored int       `json:"candidates_scored"`
	ChildrenIDs      []string  `json:"children_ids,omitempty"`
}

// TrajectoryTree represents the full exploration tree of a task session,
// containing all explored branches, dead ends, and terminal states.
type TrajectoryTree struct {
	SessionID       string                     `json:"session_id"`
	Task            string                     `json:"task"`
	RootID          string                     `json:"root_id"`
	Nodes           map[string]*TrajectoryNode `json:"nodes"`
	TotalTokens     int                        `json:"total_tokens"`
	TotalSteps      int                        `json:"total_steps"`
	BacktrackSteps  int                        `json:"backtrack_steps"`
	TerminalPassed  bool                       `json:"terminal_passed"`
	TerminalQuality float64                    `json:"terminal_quality"`
	CompletedAt     time.Time                  `json:"completed_at"`
}

// BuildTrajectoryTree converts an agent session's raw hook event sequence,
// context plan, and verification outcome into a structured exploration search tree.
func BuildTrajectoryTree(sessionID, task, revision string, events []model.HookEvent, plan model.ContextPlan, result *evaluation.Result) *TrajectoryTree {
	tree := &TrajectoryTree{
		SessionID:   sessionID,
		Task:        task,
		Nodes:       make(map[string]*TrajectoryNode),
		CompletedAt: time.Now().UTC(),
	}

	if result != nil {
		tree.TerminalPassed = result.Success
		tree.TerminalQuality = result.Quality
	}

	rootID := hashStep(sessionID, "root", 0)
	tree.RootID = rootID
	rootNode := &TrajectoryNode{
		ID:            rootID,
		SessionID:     sessionID,
		Task:          task,
		RepoRevision:  revision,
		Timestamp:     time.Now().UTC(),
		ActionTaken:   "task_init",
		BranchDepth:   0,
		ContextTokens: plan.SelectedTokens,
	}
	tree.Nodes[rootID] = rootNode

	parentID := rootID
	currentDepth := 0
	previousFailed := false

	for i, ev := range events {
		stepID := hashStep(sessionID, ev.ToolName+ev.EventType, i+1)
		isBacktrack := false

		// Invariant: consecutive failure followed by a new prompt/tool indicates a search backtrack
		if previousFailed && (!ev.IsFailure || ev.EventType == "prompt" || ev.EventType == "user_input") {
			isBacktrack = true
			tree.BacktrackSteps++
		}

		if ev.IsFailure {
			previousFailed = true
		} else {
			previousFailed = false
		}

		currentDepth++
		node := &TrajectoryNode{
			ID:               stepID,
			ParentID:         parentID,
			Timestamp:        time.Now().UTC(),
			SessionID:        sessionID,
			Task:             task,
			RepoRevision:     revision,
			ActionTaken:      ev.EventType,
			ToolName:         ev.ToolName,
			Prompt:           ev.Prompt,
			Output:           ev.Output,
			IsFailure:        ev.IsFailure,
			IsBacktrack:      isBacktrack,
			BranchDepth:      currentDepth,
			ContextTokens:    plan.SelectedTokens,
			CandidatesScored: len(plan.Candidates),
		}

		tree.Nodes[stepID] = node
		if parent, ok := tree.Nodes[parentID]; ok {
			parent.ChildrenIDs = append(parent.ChildrenIDs, stepID)
		}

		parentID = stepID
		tree.TotalSteps++
		tree.TotalTokens += plan.SelectedTokens
	}

	return tree
}

// ExplorationEfficiency computes the ratio of productive forward actions
// versus dead-end or backtracking steps. Higher is better (range [0.0, 1.0]).
func (t *TrajectoryTree) ExplorationEfficiency() float64 {
	if t.TotalSteps == 0 {
		return 1.0
	}
	productiveSteps := t.TotalSteps - t.BacktrackSteps
	if productiveSteps < 0 {
		productiveSteps = 0
	}
	return float64(productiveSteps) / float64(t.TotalSteps)
}

// GetLinearPath returns all node IDs along the principal line of exploration.
func (t *TrajectoryTree) GetLinearPath() []string {
	var path []string
	curr := t.RootID
	for curr != "" {
		path = append(path, curr)
		node, ok := t.Nodes[curr]
		if !ok || len(node.ChildrenIDs) == 0 {
			break
		}
		// Follow first child along mainline
		curr = node.ChildrenIDs[0]
	}
	return path
}

func hashStep(session, tag string, idx int) string {
	raw := fmt.Sprintf("%s|%s|%d", session, tag, idx)
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:8])
}
