package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"contextos/internal/gitidx"
	"contextos/internal/hook"
	"contextos/internal/integrations"
	"contextos/internal/report"
	"contextos/internal/router"
	"contextos/internal/server"
	"contextos/internal/store"
	"contextos/internal/ui"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		return
	}
	sub := os.Args[1]
	fs := flag.NewFlagSet(sub, flag.ExitOnError)
	repo := fs.String("repo", ".", "repository path")
	port := fs.Int("port", 8765, "HTTP server port for UI dashboard")
	dbPath := fs.String("db", "", "database or storage path")
	storage := fs.String("storage", "", "storage engine: sqlite or file (or CONTEXTOS_STORAGE)")
	autoPrune := fs.Bool("auto-prune", false, "opt-in automatic storage pruning (or CONTEXTOS_AUTO_PRUNE=1)")
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
	fromStorage := fs.String("from", "", "source storage for migration (sqlite or file)")
	toStorage := fs.String("to", "", "target storage for migration (sqlite or file)")
	fromPath := fs.String("from-path", "", "custom source storage path")
	toPath := fs.String("to-path", "", "custom target storage path")
	keepDays := fs.Int("keep-days", 30, "days to retain ephemeral records for ctx gc")
	dryRun := fs.Bool("dry-run", false, "dry-run for ctx gc")
	format := fs.String("format", "markdown", "output format for report: markdown or json")
	outputFile := fs.String("output", "", "output file path for report/publish")

	_ = fs.Parse(os.Args[2:])
	dp := *dbPath
	if dp == "" && os.Getenv("CONTEXTOS_DB") != "" {
		dp = os.Getenv("CONTEXTOS_DB")
	}
	stg := *storage
	if stg == "" && os.Getenv("CONTEXTOS_STORAGE") != "" {
		stg = os.Getenv("CONTEXTOS_STORAGE")
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
	if sub == "ui" || sub == "dashboard" {
		rp, _ := filepath.Abs(*repo)
		if dp == "" {
			dp = server.DefaultDBPath()
		}
		s, e := server.NewWithOptions(dp, rp, server.Options{
			StorageType: stg,
			AutoPrune:   *autoPrune,
		})
		if e != nil {
			die(e)
		}
		defer s.Close()
		fmt.Printf("ContextOS Dashboard starting on http://localhost:%d\n", *port)
		if err := ui.StartServer(s, rp, *port); err != nil {
			die(err)
		}
		return
	}
	if sub == "setup" {
		rp, _ := filepath.Abs(*repo)
		if dp == "" {
			dp = server.DefaultDBPath()
		}
		s, e := server.NewWithOptions(dp, rp, server.Options{
			StorageType: stg,
			AutoPrune:   *autoPrune,
		})
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
		for _, a := range []string{"claude", "cursor", "codex", "gemini", "antigravity"} {
			res, e := integrations.Install(a, rp, hookBin, mcpBin)
			if e != nil {
				die(e)
			}
			results = append(results, res)
		}
		printJSON(map[string]any{"ok": true, "repo": rp, "storage": stg, "integrations": results})
		return
	}
	if sub == "install" {
		a := *agent
		if a == "" {
			die(fmt.Errorf("-agent is required (claude|cursor|codex|gemini|antigravity|all)"))
		}
		rp, _ := filepath.Abs(*repo)
		bin, _ := os.Executable()
		hookBin := filepath.Join(filepath.Dir(bin), "ctx-hook")
		mcpBin := filepath.Join(filepath.Dir(bin), "contextd")
		if strings.EqualFold(a, "all") {
			var results []any
			for _, name := range []string{"claude", "cursor", "codex", "gemini", "antigravity"} {
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

	// Subcommand: migrate (transfers data between storage engines)
	if sub == "migrate" {
		dstType := strings.ToLower(*toStorage)
		if dstType == "" {
			die(fmt.Errorf("-to is required (sqlite or file)"))
		}
		srcType := strings.ToLower(*fromStorage)
		if srcType == "" {
			if dstType == "file" {
				srcType = "sqlite"
			} else {
				srcType = "file"
			}
		}
		srcP := *fromPath
		if srcP == "" {
			if srcType == "sqlite" {
				srcP = dp
			} else {
				srcP = filepath.Join(filepath.Dir(dp), "data")
			}
		}
		dstP := *toPath
		if dstP == "" {
			if dstType == "file" {
				dstP = filepath.Join(filepath.Dir(dp), "data")
			} else {
				dstP = dp
			}
		}

		var srcStore, dstStore store.Store
		if srcType == "sqlite" {
			var err error
			srcStore, err = store.NewSQLiteStore(srcP)
			if err != nil {
				die(fmt.Errorf("open source sqlite store %s: %w", srcP, err))
			}
		} else {
			var err error
			srcStore, err = store.NewFileStore(srcP)
			if err != nil {
				die(fmt.Errorf("open source file store %s: %w", srcP, err))
			}
		}
		defer srcStore.Close()

		if dstType == "file" {
			var err error
			dstStore, err = store.NewFileStore(dstP)
			if err != nil {
				die(fmt.Errorf("open destination file store %s: %w", dstP, err))
			}
		} else {
			var err error
			dstStore, err = store.NewSQLiteStore(dstP)
			if err != nil {
				die(fmt.Errorf("open destination sqlite store %s: %w", dstP, err))
			}
		}
		defer dstStore.Close()

		repObj, err := gitidx.Detect(rp)
		if err != nil {
			die(err)
		}

		rep, err := store.Migrate(srcStore, dstStore, repObj)
		if err != nil {
			die(err)
		}
		printJSON(rep)
		return
	}

	s, e := server.NewWithOptions(dp, rp, server.Options{
		StorageType: stg,
		AutoPrune:   *autoPrune,
	})
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
	case "gc":
		rep, err := s.GC(store.PruneOptions{
			RepoID:          s.RepoID,
			CurrentRevision: s.Repo.Revision,
			CacheTTL:        time.Duration(*keepDays) * 24 * time.Hour,
			MaxTraces:       500,
			EventTTL:        time.Duration(*keepDays) * 24 * time.Hour,
			DryRun:          *dryRun,
		})
		if err != nil {
			die(err)
		}
		printJSON(rep)
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
	case "report", "publish":
		rep, err := report.Generate(s)
		if err != nil {
			die(err)
		}
		var content string
		if strings.ToLower(*format) == "json" {
			var err error
			content, err = rep.ToJSON()
			if err != nil {
				die(err)
			}
		} else {
			content = rep.ToMarkdown()
		}

		if *outputFile != "" {
			outPath := *outputFile
			if !filepath.IsAbs(outPath) {
				outPath = filepath.Join(rp, outPath)
			}
			if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
				die(fmt.Errorf("write report to %s: %w", outPath, err))
			}
			fmt.Printf("Benchmark report successfully published to: %s\n", outPath)
		} else {
			fmt.Println(content)
		}
	default:
		usage()
	}
}

func usage() {
	fmt.Println(`ContextOS CLI

Usage:
  ctx <command> [flags]

Commands:
  ctx init|index -repo PATH                             Index repository symbols
  ctx remember   -repo PATH -kind K -content '...'      Persist a memory
  ctx plan       -repo PATH -task '...' -budget 4000    Build context plan
  ctx resume     -repo PATH                             Recover active work state
  ctx handoff    -repo PATH -task '...' -target AGENT   Cross-agent context handoff
  ctx invalidate -repo PATH -id MEMORY_ID               Invalidate stale memory
  ctx gc         -repo PATH [-keep-days 30] [-dry-run]  Garbage collect expired cache & traces
  ctx migrate    -to file|sqlite [-from file|sqlite]    Transfer data between storage engines
  ctx work       -repo PATH -title "..."                Start a work item
  ctx session    -repo PATH -agent NAME                 Start an agent session
  ctx event      -repo PATH -event TYPE -payload JSON   Record lifecycle event
  ctx stats      -repo PATH                             Display runtime statistics
  ctx route      -task "..." -budget 4000               Recommend optimal model
  ctx install    -repo PATH -agent NAME                 Install agent hook & MCP
  ctx setup      -repo PATH                             Index + install all integrations
  ctx report     -repo PATH [-format md|json] [-output] Generate evaluation/benchmark report
  ctx publish    -repo PATH [-output FILE]              Publish empirical test results to markdown
  ctx ui         -repo PATH [-port 8765]                Launch real-time web UI dashboard

Storage & Feature Flags:
  -storage sqlite|file   Choose storage engine (default: sqlite, or file for zero-DB)
  -auto-prune            Opt-in automatic pruning during context planning (default: false)
  -db PATH               Custom database or storage data path`)
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func die(e error) {
	fmt.Fprintln(os.Stderr, "contextos:", e)
	os.Exit(1)
}
