package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"contextos/internal/server"
)

// Request conforms to standard JSON-RPC 2.0 with optional MCP _meta.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	Meta    map[string]any  `json:"_meta,omitempty"`
}

// RPCError defines a standard JSON-RPC 2.0 error object.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Response conforms strictly to JSON-RPC 2.0 standard.
type Response struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type ResourceDefinition struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType,omitempty"`
}

type MCP struct {
	S *server.Service
}

func New(s *server.Service) *MCP {
	return &MCP{S: s}
}

// Run executes the MCP JSON-RPC protocol loop over in/out streams.
func (m *MCP) Run(in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 4096), 8*1024*1024)

	var debugLog *os.File
	if logPath := os.Getenv("CONTEXTOS_DEBUG_LOG"); logPath != "" {
		debugLog, _ = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	}
	defer func() {
		if debugLog != nil {
			_ = debugLog.Close()
		}
	}()

	for sc.Scan() {
		raw := sc.Bytes()
		if len(strings.TrimSpace(string(raw))) == 0 {
			continue
		}
		if debugLog != nil {
			fmt.Fprintf(debugLog, "RECV: %s\n", string(raw))
		}

		var q Request
		if err := json.Unmarshal(raw, &q); err != nil {
			if debugLog != nil {
				fmt.Fprintf(debugLog, "PARSE_ERR: %v\n", err)
			}
			resp := errParse(nil, "Parse error: "+err.Error())
			b, _ := json.Marshal(resp)
			fmt.Fprintln(out, string(b))
			continue
		}

		resp := m.Handle(q)

		// Notifications (missing or nil ID) must never elicit a response per JSON-RPC 2.0
		if q.ID == nil {
			if debugLog != nil {
				fmt.Fprintf(debugLog, "NOTIFICATION_IGNORED: %s\n", q.Method)
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
	x := map[string]any{
		"type":       "object",
		"properties": props,
	}
	if len(required) > 0 {
		x["required"] = required
	} else {
		x["required"] = []string{}
	}
	return x
}

func prop(t string) map[string]any {
	return map[string]any{"type": t}
}

// allTools returns the deterministic, canonically sorted list of ContextOS tools.
func allTools() []ToolDefinition {
	tools := []ToolDefinition{
		{
			Name:        "context_event",
			Description: "Record an agent/tool event",
			InputSchema: schema(map[string]any{"session_id": prop("string"), "event_type": prop("string"), "payload": prop("string")}, "event_type", "payload"),
		},
		{
			Name:        "context_handoff",
			Description: "Produce cross-agent handoff context",
			InputSchema: schema(map[string]any{"task": prop("string"), "target_model": prop("string"), "budget": prop("integer")}, "task"),
		},
		{
			Name:        "context_invalidate",
			Description: "Invalidate a memory at the current repository revision",
			InputSchema: schema(map[string]any{"id": prop("string")}, "id"),
		},
		{
			Name:        "context_plan",
			Description: "Select minimum-sufficient, cache-aware context under a token budget with adaptive timeout mitigation",
			InputSchema: schema(map[string]any{
				"task":            prop("string"),
				"model":           prop("string"),
				"budget":          prop("integer"),
				"timeout_ms":      prop("integer"),
				"adaptive_budget": prop("boolean"),
			}, "task"),
		},
		{
			Name:        "context_remember",
			Description: "Persist durable engineering knowledge",
			InputSchema: schema(map[string]any{"kind": prop("string"), "content": prop("string"), "authority": prop("string")}, "kind", "content"),
		},
		{
			Name:        "context_resume",
			Description: "Recover latest repository/work state",
			InputSchema: schema(map[string]any{}),
		},
		{
			Name:        "context_route",
			Description: "Recommend a model using task complexity and budget",
			InputSchema: schema(map[string]any{"task": prop("string"), "budget": prop("integer")}, "task"),
		},
		{
			Name:        "context_search",
			Description: "Search durable engineering memory and repository context",
			InputSchema: schema(map[string]any{
				"task":       prop("string"),
				"limit":      prop("integer"),
				"timeout_ms": prop("integer"),
			}, "task"),
		},
		{
			Name:        "context_session_start",
			Description: "Start an agent session",
			InputSchema: schema(map[string]any{"agent": prop("string"), "work_item": prop("string")}),
		},
		{
			Name:        "context_stats",
			Description: "Show ContextOS statistics",
			InputSchema: schema(map[string]any{}),
		},
		{
			Name:        "context_trace",
			Description: "Show latest context allocation trace",
			InputSchema: schema(map[string]any{}),
		},
		{
			Name:        "context_work_start",
			Description: "Start a persistent engineering work item",
			InputSchema: schema(map[string]any{"title": prop("string")}, "title"),
		},
	}
	// Guarantee deterministic alphabetical ordering
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

// allResources returns the deterministic, canonically sorted list of ContextOS resources.
func allResources() []ResourceDefinition {
	res := []ResourceDefinition{
		{URI: "context://memory/relevant", Name: "Relevant memory", Description: "Top durable memories relevant to recent context"},
		{URI: "context://repo/current", Name: "Current repository", Description: "Current repository metadata, revision, and branch"},
		{URI: "context://session/latest", Name: "Latest session", Description: "Most recent agent session context"},
		{URI: "context://trace/latest", Name: "Latest context trace", Description: "Most recent context allocation plan and decisions"},
		{URI: "context://work-item/current", Name: "Current work item", Description: "Active engineering work item details"},
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].URI < res[j].URI
	})
	return res
}

// Handle routes and dispatches JSON-RPC requests conforming to MCP specifications.
func (m *MCP) Handle(q Request) Response {
	switch q.Method {
	// Step 1: server/discover (Modern stateless MCP 2026-07-28+)
	case "server/discover":
		return ok(q.ID, map[string]any{
			"supportedVersions": []string{"2026-07-28", "2024-11-05"},
			"capabilities": map[string]any{
				"tools": map[string]any{
					"listChanged": false,
				},
				"resources": map[string]any{
					"subscribe":   false,
					"listChanged": false,
				},
			},
			"serverInfo": map[string]any{
				"name":    "contextos",
				"version": "0.7.0",
			},
			"instructions": "ContextOS provides persistent agent context, 6-pass token allocations, durable engineering memory, and session traces.",
			"ttl":          3600,
			"cacheScope":   "session",
		})

	// Step 3: initialize (Backward compatibility for legacy initialize handshake)
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
				"tools": map[string]any{
					"listChanged": false,
				},
				"resources": map[string]any{
					"subscribe":   false,
					"listChanged": false,
				},
			},
			"serverInfo": map[string]any{
				"name":    "contextos",
				"version": "0.7.0",
			},
		})

	case "notifications/initialized", "initialized":
		return Response{JSONRPC: "2.0"}

	case "ping":
		return ok(q.ID, map[string]any{})

	// Step 4 & 5: tools/list with deterministic ordering and pagination support
	case "tools/list":
		tools := allTools()

		var params struct {
			Cursor string `json:"cursor"`
			Limit  int    `json:"limit"`
		}
		if len(q.Params) > 0 {
			_ = json.Unmarshal(q.Params, &params)
		}

		offset := 0
		if params.Cursor != "" {
			if parsed, err := strconv.Atoi(params.Cursor); err == nil && parsed >= 0 && parsed < len(tools) {
				offset = parsed
			}
		}

		limit := len(tools)
		if params.Limit > 0 && params.Limit < limit {
			limit = params.Limit
		}

		end := offset + limit
		nextCursor := ""
		if end < len(tools) {
			nextCursor = strconv.Itoa(end)
		} else {
			end = len(tools)
		}

		result := map[string]any{
			"tools": tools[offset:end],
		}
		if nextCursor != "" {
			result["nextCursor"] = nextCursor
		}
		return ok(q.ID, result)

	// Step 4: resources/list with deterministic ordering
	case "resources/list":
		res := allResources()
		return ok(q.ID, map[string]any{
			"resources": res,
		})

	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			return errInvalidParams(q.ID, "Invalid params: "+err.Error())
		}
		a := p.Arguments
		switch p.Name {
		case "context_search":
			task, _ := a["task"].(string)
			lim := 100
			if x, ok := a["limit"].(float64); ok {
				lim = int(x)
			}
			if tms, ok := a["timeout_ms"].(float64); ok && tms > 0 {
				oldTimeout := m.S.Timeout
				m.S.Timeout = time.Duration(tms) * time.Millisecond
				defer func() { m.S.Timeout = oldTimeout }()
			}
			v, e := m.S.SearchCandidates(task, lim)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_plan":
			task, _ := a["task"].(string)
			mn, _ := a["model"].(string)
			budget := 0
			if x, ok := a["budget"].(float64); ok {
				budget = int(x)
			}
			if tms, ok := a["timeout_ms"].(float64); ok && tms > 0 {
				oldTimeout := m.S.Timeout
				m.S.Timeout = time.Duration(tms) * time.Millisecond
				defer func() { m.S.Timeout = oldTimeout }()
			}
			if ab, ok := a["adaptive_budget"].(bool); ok {
				oldAB := m.S.AdaptiveTimeout
				m.S.AdaptiveTimeout = ab
				defer func() { m.S.AdaptiveTimeout = oldAB }()
			}
			v, e := m.S.Plan(task, mn, budget)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_resume":
			v, e := m.S.Resume()
			if e != nil {
				return errInternal(q.ID, e.Error())
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
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_remember":
			kind, _ := a["kind"].(string)
			content, _ := a["content"].(string)
			auth, _ := a["authority"].(string)
			v, e := m.S.Remember(kind, content, auth, "repo", "", 0.8, nil)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_invalidate":
			id, _ := a["id"].(string)
			if e := m.S.Invalidate(id); e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, map[string]any{"invalidated": id})

		case "context_trace":
			v, e := m.S.LatestTrace()
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_stats":
			v, e := m.S.Stats()
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_work_start":
			title, _ := a["title"].(string)
			v, e := m.S.StartWorkItem(title)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_session_start":
			agent, _ := a["agent"].(string)
			work, _ := a["work_item"].(string)
			v, e := m.S.StartSession(agent, work)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)

		case "context_event":
			sid, _ := a["session_id"].(string)
			et, _ := a["event_type"].(string)
			payload, _ := a["payload"].(string)
			if e := m.S.RecordEvent(sid, et, payload); e != nil {
				return errInternal(q.ID, e.Error())
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
			return errMethodNotFound(q.ID, fmt.Sprintf("Unknown tool: %s", p.Name))
		}

	case "resources/read":
		var p struct {
			URI string `json:"uri"`
		}
		if err := json.Unmarshal(q.Params, &p); err != nil {
			return errInvalidParams(q.ID, "Invalid params: "+err.Error())
		}
		switch p.URI {
		case "context://repo/current":
			return textOK(q.ID, m.S.Repo)
		case "context://memory/relevant":
			v, e := m.S.SearchCandidates("", 50)
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://work-item/current":
			v, e := m.S.CurrentWorkItem()
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://session/latest":
			v, e := m.S.LatestSession()
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		case "context://trace/latest":
			v, e := m.S.LatestTrace()
			if e != nil {
				return errInternal(q.ID, e.Error())
			}
			return textOK(q.ID, v)
		default:
			return errInvalidParams(q.ID, fmt.Sprintf("Resource not found: %s", p.URI))
		}

	default:
		if strings.HasPrefix(q.Method, "notifications/") {
			return Response{JSONRPC: "2.0"}
		}
		return errMethodNotFound(q.ID, fmt.Sprintf("Method not found: %s", q.Method))
	}
}

func textOK(id any, v any) Response {
	return ok(id, map[string]any{"content": []any{map[string]any{"type": "text", "text": must(v)}}})
}

func ok(id any, v any) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  v,
	}
}

func errParse(id any, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    -32700,
			Message: msg,
		},
	}
}

func errInvalidRequest(id any, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    -32600,
			Message: msg,
		},
	}
}

func errMethodNotFound(id any, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    -32601,
			Message: msg,
		},
	}
}

func errInvalidParams(id any, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    -32602,
			Message: msg,
		},
	}
}

func errInternal(id any, msg string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &RPCError{
			Code:    -32603,
			Message: msg,
		},
	}
}

func must(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
