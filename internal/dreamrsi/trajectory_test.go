package dreamrsi

import (
	"testing"
	"contextos/internal/evaluation"
	"contextos/internal/model"
)

func TestBuildTrajectoryTree(t *testing.T) {
	events := []model.HookEvent{
		{
			EventType: "tool_use",
			ToolName:  "view_file",
			Prompt:    "view internal/store/sqlite_store.go",
			Output:    "package store...",
			IsFailure: false,
		},
		{
			EventType: "tool_use",
			ToolName:  "run_command",
			Prompt:    "go test ./...",
			Output:    "FAIL: compilation error",
			IsFailure: true,
		},
		{
			EventType: "user_input",
			Prompt:    "fix syntax error in test",
			Output:    "ok",
			IsFailure: false,
		},
		{
			EventType: "tool_use",
			ToolName:  "run_command",
			Prompt:    "go test ./...",
			Output:    "PASS",
			IsFailure: false,
		},
	}

	plan := model.ContextPlan{
		SelectedTokens: 120,
		Candidates:     make([]model.Candidate, 5),
	}

	res := &evaluation.Result{
		Success: true,
		Quality: 0.95,
	}

	tree := BuildTrajectoryTree("sess-101", "fix bug in store", "rev-abc", events, plan, res)

	if tree.SessionID != "sess-101" {
		t.Fatalf("expected SessionID sess-101, got %s", tree.SessionID)
	}
	if tree.TotalSteps != 4 {
		t.Fatalf("expected TotalSteps=4, got %d", tree.TotalSteps)
	}
	if tree.BacktrackSteps != 1 {
		t.Fatalf("expected BacktrackSteps=1 (after failure), got %d", tree.BacktrackSteps)
	}
	if !tree.TerminalPassed {
		t.Fatalf("expected TerminalPassed=true")
	}
	if tree.TerminalQuality != 0.95 {
		t.Fatalf("expected TerminalQuality=0.95, got %f", tree.TerminalQuality)
	}

	eff := tree.ExplorationEfficiency()
	// 4 steps, 1 backtrack -> 3/4 = 0.75
	if eff != 0.75 {
		t.Fatalf("expected ExplorationEfficiency 0.75, got %f", eff)
	}

	path := tree.GetLinearPath()
	if len(path) != 5 { // root + 4 steps
		t.Fatalf("expected path length 5, got %d", len(path))
	}
}
