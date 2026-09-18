package ui

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"contextos/internal/integrations"
	"contextos/internal/server"
)

//go:embed assets/*
var assetsFS embed.FS

// Server wraps the HTTP API and web frontend for ContextOS.
type Server struct {
	svc      *server.Service
	repoPath string
	port     int
}

// NewServer creates a new UI server instance.
func NewServer(svc *server.Service, repoPath string, port int) *Server {
	return &Server{
		svc:      svc,
		repoPath: repoPath,
		port:     port,
	}
}

// Handler returns the HTTP handler for the UI server.
func (srv *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	sub, err := fs.Sub(assetsFS, "assets")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	// Static assets
	mux.Handle("/", fileServer)

	// API routes
	mux.HandleFunc("/api/status", srv.handleStatus)
	mux.HandleFunc("/api/memories", srv.handleMemories)
	mux.HandleFunc("/api/memories/invalidate", srv.handleInvalidateMemory)
	mux.HandleFunc("/api/plan", srv.handlePlan)
	mux.HandleFunc("/api/work-item", srv.handleWorkItem)
	mux.HandleFunc("/api/sessions", srv.handleSessions)
	mux.HandleFunc("/api/integrations", srv.handleIntegrations)
	mux.HandleFunc("/api/install", srv.handleInstall)

	return mux
}

// StartServer starts the HTTP server on the specified port.
func StartServer(svc *server.Service, repoPath string, port int) error {
	srv := NewServer(svc, repoPath, port)
	addr := fmt.Sprintf(":%d", port)
	return http.ListenAndServe(addr, srv.Handler())
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}

func (srv *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	stats, err := srv.svc.Stats()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	wi, _ := srv.svc.CurrentWorkItem()
	sess, _ := srv.svc.LatestSession()

	stg := "sqlite"
	if srv.svc.Store != nil {
		if _, ok := stats["storage_engine"]; ok {
			stg = fmt.Sprint(stats["storage_engine"])
		}
	}

	mems, _ := srv.svc.Store.ListMemories(srv.svc.RepoID, 10000)
	var decisions, failures, constraints, facts, codes int
	for _, m := range mems {
		switch m.Kind {
		case "decision":
			decisions++
		case "failure":
			failures++
		case "constraint":
			constraints++
		case "fact":
			facts++
		case "code":
			codes++
		}
	}
	stats["decisions"] = decisions
	stats["failures"] = failures
	stats["constraints"] = constraints
	stats["facts"] = facts
	stats["codes"] = codes
	stats["memories"] = len(mems)

	jsonResponse(w, http.StatusOK, map[string]any{
		"repo": map[string]any{
			"branch":        srv.svc.Repo.Branch,
			"revision":      srv.svc.Repo.Revision,
			"path":          srv.svc.Repo.Path,
			"worktree_hash": srv.svc.Repo.WorktreeHash,
		},
		"repo_id":        srv.svc.RepoID,
		"storage":        stg,
		"stats":          stats,
		"work_item":      wi,
		"latest_session": sess,
	})
}

func (srv *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 500
		q := r.URL.Query().Get("q")
		if q != "" {
			mems, err := srv.svc.SearchCandidates(q, limit)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, mems)
			return
		}
		mems, err := srv.svc.Store.ListMemories(srv.svc.RepoID, limit)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, mems)

	case http.MethodPost:
		var req struct {
			Kind       string   `json:"kind"`
			Content    string   `json:"content"`
			Authority  string   `json:"authority"`
			Scope      string   `json:"scope"`
			WorkID     string   `json:"work_id"`
			Confidence float64  `json:"confidence"`
			Provenance []string `json:"provenance"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		if strings.TrimSpace(req.Content) == "" {
			jsonError(w, http.StatusBadRequest, "content is required")
			return
		}
		if req.Kind == "" {
			req.Kind = "decision"
		}
		if req.Authority == "" {
			req.Authority = "user"
		}
		if req.Confidence <= 0 {
			req.Confidence = 1.0
		}
		mem, err := srv.svc.Remember(req.Kind, req.Content, req.Authority, req.Scope, req.WorkID, req.Confidence, req.Provenance)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, mem)

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (srv *Server) handleInvalidateMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		jsonError(w, http.StatusBadRequest, "id is required")
		return
	}
	if err := srv.svc.Invalidate(req.ID); err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]bool{"ok": true})
}

func (srv *Server) handlePlan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Task   string `json:"task"`
		Model  string `json:"model"`
		Budget int    `json:"budget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Task) == "" {
		req.Task = "General development and engineering tasks"
	}
	if req.Budget <= 0 {
		req.Budget = 2500
	}
	plan, err := srv.svc.Plan(req.Task, req.Model, req.Budget)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	rendered := srv.svc.RenderPlan(plan)
	jsonResponse(w, http.StatusOK, map[string]any{
		"plan":     plan,
		"rendered": rendered,
	})
}

func (srv *Server) handleWorkItem(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		wi, err := srv.svc.CurrentWorkItem()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, wi)

	case http.MethodPost:
		var req struct {
			Title string `json:"title"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Title) == "" {
			jsonError(w, http.StatusBadRequest, "title is required")
			return
		}
		wi, err := srv.svc.StartWorkItem(req.Title)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		jsonResponse(w, http.StatusCreated, wi)

	default:
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (srv *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	sessions, err := srv.svc.Store.ListSessions(srv.svc.RepoID)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	events, err := srv.svc.Store.ListEvents("", 50)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"sessions": sessions,
		"events":   events,
	})
}

func (srv *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	rp := srv.repoPath
	status := map[string]any{
		"antigravity": map[string]any{
			"installed": fileExists(filepath.Join(rp, ".agents", "mcp_config.json")) && fileExists(filepath.Join(rp, ".agents", "hooks.json")),
			"files": []string{
				filepath.Join(".agents", "mcp_config.json"),
				filepath.Join(".agents", "hooks.json"),
				filepath.Join(".agents", "rules", "contextos.md"),
			},
		},
		"cursor": map[string]any{
			"installed": fileExists(filepath.Join(rp, ".cursor", "mcp.json")) && fileExists(filepath.Join(rp, ".cursor", "hooks.json")),
			"files": []string{
				filepath.Join(".cursor", "mcp.json"),
				filepath.Join(".cursor", "hooks.json"),
				filepath.Join(".cursor", "rules", "contextos.mdc"),
			},
		},
		"claude": map[string]any{
			"installed": fileExists(filepath.Join(rp, ".claude", "settings.local.json")) && fileExists(filepath.Join(rp, ".mcp.json")),
			"files": []string{
				filepath.Join(".claude", "settings.local.json"),
				".mcp.json",
			},
		},
		"codex": map[string]any{
			"installed": fileExists(filepath.Join(rp, ".codex", "config.toml")) && fileExists(filepath.Join(rp, ".codex", "hooks.json")),
			"files": []string{
				filepath.Join(".codex", "config.toml"),
				filepath.Join(".codex", "hooks.json"),
			},
		},
		"gemini": map[string]any{
			"installed": fileExists(filepath.Join(rp, ".gemini", "settings.json")),
			"files": []string{
				filepath.Join(".gemini", "settings.json"),
			},
		},
	}
	jsonResponse(w, http.StatusOK, status)
}

func (srv *Server) handleInstall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Agent string `json:"agent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Agent == "" {
		jsonError(w, http.StatusBadRequest, "agent is required")
		return
	}

	bin, _ := os.Executable()
	hookBin := filepath.Join(filepath.Dir(bin), "ctx-hook")
	mcpBin := filepath.Join(filepath.Dir(bin), "contextd")

	res, err := integrations.Install(req.Agent, srv.repoPath, hookBin, mcpBin)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonResponse(w, http.StatusOK, res)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// DrainReader is a helper for testing
func DrainReader(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
