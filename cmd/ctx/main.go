package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"contextos/internal/hook"
	"contextos/internal/integrations"
	"contextos/internal/router"
	"contextos/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	sub := os.Args[1]
	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	dbPath := fs.String("db", "", "SQLite database path")
	task := fs.String("task", "", "task text")
	modelName := fs.String("model", "", "model name")
	budget := fs.Int("budget", 4000, "token budget")
	kind := fs.String("kind", "fact", "memory kind")
	content := fs.String("content", "", "memory content")
	authority := fs.String("authority", "inference", "memory authority")
	id := fs.String("id", "", "memory id")
	target := fs.String("target", "", "handoff target model")
	agent := fs.String("agent", "", "agent/session name")
	sessionID := fs.String("session", "", "session id")
	eventType := fs.String("event", "", "event type")
	title := fs.String("title", "", "work item title")
	payload := fs.String("payload", "", "event payload")
	render := fs.Bool("render", false, "render plan as context text")
	dp := *dbPath
	_ = fs.Parse(os.Args[2:])
	dp = *dbPath
	if dp == "" && os.Getenv("CONTEXTOS_DB") != "" {
		dp = os.Getenv("CONTEXTOS_DB")
	}

	if sub == "hook" {
		raw, e := io.ReadAll(os.Stdin)
		if e != nil {
			die(e)
		}
		out, e := hook.Handle(*agent, *eventType, raw)
		if e != nil {
			die(e)
		}
		// Do not print anything but JSON to stdout: required by supported hooks.
		fmt.Fprintln(os.Stdout, string(out))
		return
	}
	if sub == "setup" {
		rp, _ := filepath.Abs(*repo)
		if dp == "" {
			dp = server.DefaultDBPath()
		}
		s, e := server.New(dp, rp)
		if e != nil {
			die(e)
		}
		if e = s.Index(); e != nil {
			s.Close()
			die(e)
		}
		s.Close()
		bin, _ := os.Executable()
		hookBin := filepath.Join(filepath.Dir(bin), "ctx-hook")
		mcpBin := filepath.Join(filepath.Dir(bin), "contextd")
		var results []any
		for _, a := range []string{"claude", "cursor", "codex", "gemini"} {
			res, e := integrations.Install(a, rp, hookBin, mcpBin)
			if e != nil {
				die(e)
			}
			results = append(results, res)
		}
		printJSON(map[string]any{"ok": true, "repo": rp, "integrations": results})
		return
	}
	if sub == "install" {
		a := *agent
		if a == "" {
			die(fmt.Errorf("-agent is required (claude|cursor|codex|gemini|all)"))
		}
		rp, _ := filepath.Abs(*repo)
		bin, _ := os.Executable()
		hookBin := filepath.Join(filepath.Dir(bin), "ctx-hook")
		mcpBin := filepath.Join(filepath.Dir(bin), "contextd")
		if strings.EqualFold(a, "all") {
			var results []any
			for _, name := range []string{"claude", "cursor", "codex", "gemini"} {
				res, e := integrations.Install(name, rp, hookBin, mcpBin)
				if e != nil {
					die(e)
				}
				results = append(results, res)
			}
			printJSON(results)
			return
		}
		res, e := integrations.Install(strings.ToLower(a), rp, hookBin, mcpBin)
		if e != nil {
			die(e)
		}
		printJSON(res)
		return
	}
	if sub == "route" {
		if *task == "" {
			die(fmt.Errorf("-task is required"))
		}
		p := router.Recommend(*task, *budget)
		printJSON(p)
		return
	}
	rp, _ := filepath.Abs(*repo)
	if dp == "" {
		dp = server.DefaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(dp), 0700); err != nil {
		dp = filepath.Join(".contextos", "context.db")
		_ = os.MkdirAll(filepath.Dir(dp), 0700)
	}
	s, e := server.New(dp, rp)
	if e != nil {
		die(e)
	}
	defer s.Close()
	switch sub {
	case "init", "index":
		if e := s.Index(); e != nil {
			die(e)
		}
		st, e := s.Stats()
		if e != nil {
			die(e)
		}
		printJSON(map[string]any{"ok": true, "stats": st})
	case "remember":
		if strings.TrimSpace(*content) == "" {
			die(fmt.Errorf("-content is required"))
		}
		m, e := s.Remember(*kind, *content, *authority, "repo", "", 0.8, nil)
		if e != nil {
			die(e)
		}
		printJSON(m)
	case "plan":
		if *task == "" {
			die(fmt.Errorf("-task is required"))
		}
		p, e := s.Plan(*task, *modelName, *budget)
		if e != nil {
			die(e)
		}
		if *render {
			fmt.Print(s.RenderPlan(p))
		} else {
			printJSON(p)
		}
	case "resume":
		x, e := s.Resume()
		if e != nil {
			die(e)
		}
		printJSON(x)
	case "handoff":
		if *task == "" || *target == "" {
			die(fmt.Errorf("-task and -target are required"))
		}
		x, e := s.Handoff(*task, *target, *budget)
		if e != nil {
			die(e)
		}
		printJSON(x)
	case "invalidate":
		if *id == "" {
			die(fmt.Errorf("-id is required"))
		}
		if e := s.Invalidate(*id); e != nil {
			die(e)
		}
		printJSON(map[string]any{"ok": true, "id": *id})
	case "stats":
		x, e := s.Stats()
		if e != nil {
			die(e)
		}
		printJSON(x)
	case "work":
		if *title == "" {
			die(fmt.Errorf("-title is required"))
		}
		w, e := s.StartWorkItem(*title)
		if e != nil {
			die(e)
		}
		printJSON(w)
	case "session":
		ss, e := s.StartSession(*agent, "")
		if e != nil {
			die(e)
		}
		printJSON(ss)
	case "event":
		if *eventType == "" || *payload == "" {
			die(fmt.Errorf("-event and -payload are required"))
		}
		if e := s.RecordEvent(*sessionID, *eventType, *payload); e != nil {
			die(e)
		}
		printJSON(map[string]any{"ok": true})
	default:
		usage()
	}
}
func usage() {
	fmt.Println(`ContextOS CLI

Commands:
  ctx init|index -repo PATH
  ctx remember -repo PATH -kind decision -content '...'
  ctx plan -repo PATH -task '...' [-model MODEL] -budget 4000 [-render]
  ctx resume -repo PATH
  ctx handoff -repo PATH -task '...' -target codex -budget 4000
  ctx invalidate -repo PATH -id MEMORY_ID
  ctx work -repo PATH -title "Implement failover"
  ctx session -repo PATH -agent codex
  ctx event -repo PATH -session SESSION_ID -event tool_call -payload '{"tool":"git"}'
  ctx stats -repo PATH
  ctx route -task "complex distributed debugging" -budget 4000
  ctx install -repo PATH -agent claude|cursor|codex|gemini|all
  ctx setup -repo PATH    # index repo + install all supported integrations
  ctx hook -agent claude -event UserPromptSubmit < hook.json`)
}
func printJSON(v any) { b, _ := json.MarshalIndent(v, "", "  "); fmt.Println(string(b)) }
func die(e error)     { fmt.Fprintln(os.Stderr, "contextos:", e); os.Exit(1) }
