package hook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"contextos/internal/model"
	"contextos/internal/server"
)

type Payload struct{ M map[string]any }

func readString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
			if v != nil {
				return fmt.Sprint(v)
			}
		}
	}
	return ""
}

func flattenText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]any:
		for _, k := range []string{"prompt", "message", "content", "output", "response", "reason", "summary", "command"} {
			if s := readString(x, k); s != "" {
				return s
			}
		}
		b, _ := json.Marshal(x)
		return string(b)
	case []any:
		var parts []string
		for _, y := range x {
			parts = append(parts, flattenText(y))
		}
		return strings.Join(parts, "\n")
	default:
		return fmt.Sprint(v)
	}
}

func Normalize(agent, event string, raw []byte) (model.HookEvent, error) {
	var m map[string]any
	if len(strings.TrimSpace(string(raw))) == 0 {
		m = map[string]any{}
	} else if err := json.Unmarshal(raw, &m); err != nil {
		return model.HookEvent{}, err
	}
	cwd := readString(m, "cwd", "working_directory", "project_dir", "GEMINI_CWD")
	session := readString(m, "session_id", "sessionId", "conversation_id", "thread_id", "GEMINI_SESSION_ID")
	prompt := readString(m, "prompt", "user_prompt", "userPrompt", "message", "prompt_text")
	if prompt == "" {
		if v, ok := m["prompt_input"]; ok {
			prompt = flattenText(v)
		}
	}
	output := readString(m, "output", "response", "tool_output", "tool_result")
	tool := readString(m, "tool_name", "tool", "name")
	failure := strings.Contains(strings.ToLower(event), "failure") || strings.Contains(strings.ToLower(event), "error")
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	return model.HookEvent{Agent: agent, EventType: event, CWD: cwd, SessionID: session, ToolName: tool, Prompt: prompt, Output: output, IsFailure: failure, Raw: m}, nil
}

func openService(cwd string) (*server.Service, error) {
	repo := cwd
	if repo == "" {
		repo = "."
	}
	repo, _ = filepath.Abs(repo)
	dbPath := server.DefaultDBPath()
	if v := os.Getenv("CONTEXTOS_DB"); v != "" {
		dbPath = v
	}
	return server.New(dbPath, repo)
}

func IngestEvent(ev model.HookEvent) error {
	s, err := openService(ev.CWD)
	if err != nil {
		return err
	}
	defer s.Close()
	return s.IngestHook(ev)
}

func Ingest(agent, event string, raw []byte) error {
	ev, err := Normalize(agent, event, raw)
	if err != nil {
		return err
	}
	return IngestEvent(ev)
}

func hookModel() string { return strings.TrimSpace(os.Getenv("CONTEXTOS_HOOK_MODEL")) }
func hookBudget() int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv("CONTEXTOS_HOOK_BUDGET"))); err == nil && v > 0 {
		return v
	}
	return 2500
}

func contextFor(s *server.Service, ev model.HookEvent) string {
	task := strings.TrimSpace(ev.Prompt)
	if task == "" {
		if wi, _ := s.CurrentWorkItem(); wi != nil {
			task = wi.Title
		}
	}
	if task == "" {
		return ""
	}
	p, err := s.Plan(task, hookModel(), hookBudget())
	if err != nil || len(p.Selected) == 0 {
		return ""
	}
	return s.RenderPlan(p)
}

func withClaude(event, additional string) map[string]any {
	out := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event}}
	if additional != "" {
		out["hookSpecificOutput"].(map[string]any)["additionalContext"] = additional
	}
	return out
}

// Handle performs ingestion and, where the host hook protocol supports it,
// returns additional context. Stdout from hooks must contain JSON only.
func Handle(agent, event string, raw []byte) ([]byte, error) {
	ev, err := Normalize(agent, event, raw)
	if err != nil {
		return nil, err
	}
	s, err := openService(ev.CWD)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if err := s.IngestHook(ev); err != nil {
		return nil, err
	}

	a := strings.ToLower(agent)
	e := strings.ToLower(event)
	ctx := ""
	if a == "claude" || a == "cursor" || a == "gemini" {
		ctx = contextFor(s, ev)
	}

	var out any = map[string]any{}
	switch a {
	case "claude":
		if e == "sessionstart" || e == "userpromptsubmit" || e == "posttoolusefailure" {
			out = withClaude(event, ctx)
		}
	case "cursor":
		if e == "sessionstart" || e == "posttoolusefailure" {
			out = map[string]any{}
			if ctx != "" {
				out = map[string]any{"additional_context": ctx}
			}
		} else if e == "beforesubmitprompt" {
			out = map[string]any{"continue": true}
		}
	case "gemini":
		if e == "sessionstart" || e == "beforeagent" {
			out = map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": event}}
			if ctx != "" {
				out.(map[string]any)["hookSpecificOutput"].(map[string]any)["additionalContext"] = ctx
			}
		}
	case "codex":
		// Codex hook output capabilities differ by release; durable capture remains
		// reliable here, while context can be requested through the MCP server.
	}
	return json.Marshal(out)
}
