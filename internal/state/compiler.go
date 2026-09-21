package state

import (
	"fmt"
	"strings"

	"contextos/internal/providers"
	"contextos/internal/textutil"
)

// CompiledStateBundle represents the compressed conversation context ready for prompt injection.
type CompiledStateBundle struct {
	StateMarkdown  string              `json:"state_markdown"`
	RecentMessages []providers.Message `json:"recent_messages"`
	OriginalTokens int                 `json:"original_tokens"`
	CompiledTokens int                 `json:"compiled_tokens"`
	CompressionRatio float64           `json:"compression_ratio"` // 1.0 - (compiled / original)
}

// ConversationCompiler compresses voluminous multi-turn transcripts into durable DecisionState.
type ConversationCompiler struct {
	maxRecentTurns int
}

// NewConversationCompiler initializes a conversation compiler with a target recent-turn window.
func NewConversationCompiler(maxRecentTurns int) *ConversationCompiler {
	if maxRecentTurns <= 0 {
		maxRecentTurns = 4 // Keep last 4 turns (user/assistant pairs)
	}
	return &ConversationCompiler{maxRecentTurns: maxRecentTurns}
}

// Compile takes raw multi-turn conversation messages and compiles them into a compact DecisionState bundle.
func (cc *ConversationCompiler) Compile(
	taskObjective string,
	messages []providers.Message,
	existingState *DecisionState,
) CompiledStateBundle {
	var originalTokens int
	for _, m := range messages {
		originalTokens += textutil.EstimateTokens(m.Content)
	}

	ds := NewDecisionState(taskObjective)
	if existingState != nil {
		ds = *existingState
	}

	// Extract facts, decisions, and constraints from message contents
	for _, m := range messages {
		text := m.Content
		lines := strings.Split(text, "\n")
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			lower := strings.ToLower(trimmed)

			// Look for fact declarations
			if strings.HasPrefix(lower, "fact:") || strings.HasPrefix(lower, "observation:") {
				content := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
				ds.AddFact(content, string(m.Role))
			} else if strings.HasPrefix(lower, "decision:") || strings.HasPrefix(lower, "decided:") {
				content := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
				ds.RecordDecision(content, "Derived from conversation", string(m.Role))
			} else if strings.HasPrefix(lower, "constraint:") || strings.HasPrefix(lower, "rule:") {
				content := strings.TrimSpace(trimmed[strings.Index(trimmed, ":")+1:])
				ds.Constraints = append(ds.Constraints, Constraint{
					ID:          content,
					Description: content,
					Source:      string(m.Role),
				})
			}
		}
	}

	// Slice recent messages
	var recent []providers.Message
	if len(messages) <= cc.maxRecentTurns {
		recent = messages
	} else {
		recent = messages[len(messages)-cc.maxRecentTurns:]
	}

	stateMarkdown := FormatDecisionStateMarkdown(ds)
	compiledTokens := textutil.EstimateTokens(stateMarkdown)
	for _, r := range recent {
		compiledTokens += textutil.EstimateTokens(r.Content)
	}

	ratio := 0.0
	if originalTokens > 0 {
		ratio = 1.0 - (float64(compiledTokens) / float64(originalTokens))
		if ratio < 0 {
			ratio = 0 // If raw transcript was shorter than state schema
		}
	}

	return CompiledStateBundle{
		StateMarkdown:    stateMarkdown,
		RecentMessages:   recent,
		OriginalTokens:   originalTokens,
		CompiledTokens:   compiledTokens,
		CompressionRatio: ratio,
	}
}

// FormatDecisionStateMarkdown serializes a DecisionState into an efficient markdown block.
func FormatDecisionStateMarkdown(ds DecisionState) string {
	var sb strings.Builder
	sb.WriteString("### ContextOS Decision State\n")
	sb.WriteString(fmt.Sprintf("**Objective**: %s\n", ds.Objective))
	sb.WriteString(fmt.Sprintf("**Confidence**: %.2f | **Residual Risk**: %.2f\n\n", ds.Confidence, ds.Risk))

	if len(ds.Constraints) > 0 {
		sb.WriteString("**Active Constraints**:\n")
		for _, c := range ds.Constraints {
			sb.WriteString(fmt.Sprintf("- %s\n", c.Description))
		}
		sb.WriteString("\n")
	}

	if len(ds.Facts) > 0 {
		sb.WriteString("**Established Facts**:\n")
		for _, f := range ds.Facts {
			sb.WriteString(fmt.Sprintf("- %s\n", f.Content))
		}
		sb.WriteString("\n")
	}

	if len(ds.Decisions) > 0 {
		sb.WriteString("**Committed Decisions**:\n")
		for _, d := range ds.Decisions {
			sb.WriteString(fmt.Sprintf("- **%s**: %s\n", d.Title, d.Rationale))
		}
		sb.WriteString("\n")
	}

	if len(ds.RejectedHypotheses) > 0 {
		sb.WriteString("**Rejected Alternatives** (Do not revisit):\n")
		for _, r := range ds.RejectedHypotheses {
			sb.WriteString(fmt.Sprintf("- ~~%s~~ (Reason: %s)\n", r.Claim, r.Reason))
		}
		sb.WriteString("\n")
	}

	if len(ds.Uncertainties) > 0 {
		sb.WriteString("**Open Uncertainties**:\n")
		for _, u := range ds.Uncertainties {
			if !u.Resolved {
				sb.WriteString(fmt.Sprintf("- [%s] %s (Action: %s)\n", u.Type, u.Description, u.RecommendedAction))
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
