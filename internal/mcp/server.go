package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"contextos/internal/server"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Meta    map[string]any  `json:"_meta,omitempty"`
}
type Response struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Result  any            `json:"result,omitempty"`
	Error   any            `json:"error,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}
type MCP struct{ S *server.Service }

func New(s *server.Service) *MCP { return &MCP{S: s} }
func (m *MCP) Run(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 4096), 8*1024*1024)
	debugLog, _ := os.OpenFile("/Users/rohitshukla/Desktop/ContextOS/data/mcp_debug.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	defer func() {
		if debugLog != nil {
			debugLog.Close()
		}
	}()
	for sc.Scan() {
		raw := sc.Bytes()
		if debugLog != nil {
			fmt.Fprintf(debugLog, "RECV: %s\n", string(raw))
		}
		var q Request
		if json.Unmarshal(raw, &q) != nil {
			if debugLog != nil {
				fmt.Fprintf(debugLog, "UNMARSHAL_ERR\n")
			}
			continue
		}
		resp := m.handle(q)
		// Per JSON-RPC 2.0, notifications (requests without an ID) must never elicit a response
		if q.ID == nil {
			if debugLog != nil {
				fmt.Fprintf(debugLog, "NOTIFICATION_IGNORED\n")
			}
			continue
		}
		b, _ := json.Marshal(resp)
		if debugLog != nil {
			fmt.Fprintf(debugLog, "SENT: %s\n", string(b))
		}
		fmt.Fprintln(out, string(b))
	}
	return sc.Err()
}
func schema(props map[string]any, required ...string) map[string]any {
	x := map[string]any{"type": "object", "properties": props}
	if len(required) > 0 {
		x["required"] = required
	}
	return x
}
func prop(t string) map[string]any { return map[string]any{"type": t} }
func (m *MCP) handle(q Request) Response {
	switch q.Method {
	case "initialize":
		ver := "2024-11-05"
		var initParams struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		if len(q.Params) > 0 {
			_ = json.Unmarshal(q.Params, &initParams)
			if initParams.ProtocolVersion != "" {
				ver = initParams.ProtocolVersion
			}
		}
		return ok(q.ID, map[string]any{
			"protocolVersion": ver,
			"capabilities": map[string]any{
				"tools":     map[string]any{},
				"resources": map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "contextos",
				"version": "0.6.0",
			},
		})
	case "notifications/initialized", "initialized":
		return Response{}
	case "ping":
		return ok(q.ID, map[string]any{})
	case "tools/list":
		tools := []any{
			map[string]any{"name": "context_search", "description": "Search durable engineering memory and repository context", "inputSchema": schema(map[string]any{"task": prop("string"), "limit": prop("integer")}, "task")},
			map[string]any{"name": "context_plan", "description": "Select minimum-sufficient, cache-aware context under a token budget", "inputSchema": schema(map[string]any{"task": prop("string"), "model": prop("string"), "budget": prop("integer")}, "task")},
			map[string]any{"name": "context_resume", "description": "Recover latest repository/work state", "inputSchema": schema(map[string]any{})},
			map[string]any{"name": "context_handoff", "description": "Produce cross-agent handoff context", "inputSchema": schema(map[string]any{"task": prop("string"), "target_model": prop("string"), "budget": prop("integer")}, "task")},
			map[string]any{"name": "context_remember", "description": "Persist durable engineering knowledge", "inputSchema": schema(map[string]any{"kind": prop("string"), "content": prop("string"), "authority": prop("string")}, "kind", "content")},
			map[string]any{"name": "context_invalidate", "description": "Invalidate a memory at the current repository revision", "inputSchema": schema(map[string]any{"id": prop("string")}, "id")},
			map[string]any{"name": "context_trace", "description": "Show latest context allocation trace", "inputSchema": schema(map[string]any{})},
			map[string]any{"name": "context_stats", "description": "Show ContextOS statistics", "inputSchema": schema(map[string]any{})},
			map[string]any{"name": "context_work_start", "description": "Start a persistent engineering work item", "inputSchema": schema(map[string]any{"title": prop("string")}, "title")},
			map[string]any{"name": "context_session_start", "description": "Start an agent session", "inputSchema": schema(map[string]any{"agent": prop("string"), "work_item": prop("string")})},
			map[string]any{"name": "context_event", "description": "Record an agent/tool event", "inputSchema": schema(map[string]any{"session_id": prop("string"), "event_type": prop("string"), "payload": prop("string")}, "event_type", "payload")},
			map[string]any{"name": "context_route", "description": "Recommend a model using task complexity and budget", "inputSchema": schema(map[string]any{"task": prop("string"), "budget": prop("integer")}, "task")},
		}
		return ok(q.ID, map[string]any{"tools": tools})
	case "resources/list":
		return ok(q.ID, map[string]any{"resources": []any{
			map[string]string{"uri": "context://repo/current", "name": "Current repository"},
			map[string]string{"uri": "context://work-item/current", "name": "Current work item"},
			map[string]string{"uri": "context://memory/relevant", "name": "Relevant memory"},
			map[string]string{"uri": "context://session/latest", "name": "Latest session"},
			map[string]string{"uri": "context://trace/latest", "name": "Latest context trace"},
		}})
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			return fail(q.ID, err.Error())
		}
		a := p.Arguments
		switch p.Name {
		case "context_search":
			task, _ := a["task"].(string)
			lim := 100
			if x, ok := a["limit"].(float64); ok {
				lim = int(x)
			}
			v, e := m.S.SearchCandidates(task, lim)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_plan":
			task, _ := a["task"].(string)
			mn, _ := a["model"].(string)
			budget := 4000
			if x, ok := a["budget"].(float64); ok {
				budget = int(x)
			}
			v, e := m.S.Plan(task, mn, budget)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_resume":
			v, e := m.S.Resume()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_handoff":
			task, _ := a["task"].(string)
			target, _ := a["target_model"].(string)
			budget := 4000
			if x, ok := a["budget"].(float64); ok {
				budget = int(x)
			}
			v, e := m.S.Handoff(task, target, budget)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_remember":
			kind, _ := a["kind"].(string)
			content, _ := a["content"].(string)
			auth, _ := a["authority"].(string)
			v, e := m.S.Remember(kind, content, auth, "repo", "", 0.8, nil)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_invalidate":
			id, _ := a["id"].(string)
			if e := m.S.Invalidate(id); e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, map[string]any{"invalidated": id})
		case "context_trace":
			v, e := m.S.LatestTrace()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_stats":
			v, e := m.S.Stats()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_work_start":
			title, _ := a["title"].(string)
			v, e := m.S.StartWorkItem(title)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_session_start":
			agent, _ := a["agent"].(string)
			work, _ := a["work_item"].(string)
			v, e := m.S.StartSession(agent, work)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context_event":
			sid, _ := a["session_id"].(string)
			et, _ := a["event_type"].(string)
			payload, _ := a["payload"].(string)
			if e := m.S.RecordEvent(sid, et, payload); e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, map[string]any{"ok": true})
		case "context_route":
			task, _ := a["task"].(string)
			budget := 4000
			if x, ok := a["budget"].(float64); ok {
				budget = int(x)
			}
			v := m.S.Route(task, budget)
			return textOK(q.ID, v)
		default:
			return fail(q.ID, "unknown tool")
		}
	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			return fail(q.ID, err.Error())
		}
		switch p.URI {
		case "context://repo/current":
			return textOK(q.ID, m.S.Repo)
		case "context://memory/relevant":
			v, e := m.S.SearchCandidates("", 50)
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://work-item/current":
			v, e := m.S.CurrentWorkItem()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://session/latest":
			v, e := m.S.LatestSession()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://trace/latest":
			v, e := m.S.LatestTrace()
			if e != nil {
				return fail(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		default:
			return fail(q.ID, "resource not found")
		}
	default:
		if strings.HasPrefix(q.Method, "notifications/") {
			return Response{}
		}
		return fail(q.ID, strings.TrimSpace(q.Method)+" not implemented")
	}
}
func textOK(id any, v any) Response {
	return ok(id, map[string]any{"content": []any{map[string]any{"type": "text", "text": must(v)}}})
}
func ok(id any, v any) Response {
	return Response{JSONRPC: "2.0", ID: id, Result: v, Meta: map[string]any{"io.modelcontextprotocol/serverInfo": map[string]string{"name": "contextos", "version": "0.6.0"}}}
}
func fail(id any, msg string) Response {
	return Response{JSONRPC: "2.0", ID: id, Error: map[string]any{"code": -32000, "message": msg}}
}
func must(v any) string { b, _ := json.Marshal(v); return string(b) }
