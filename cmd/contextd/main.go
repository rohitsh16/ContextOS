package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"contextos/internal/mcp"
	"contextos/internal/server"
	"contextos/internal/ui"
)

func main() {
	repo := flag.String("repo", ".", "repository path")
	dbPath := flag.String("db", "", "SQLite or file store database/data path")
	storage := flag.String("storage", "", "storage engine: sqlite or file (or CONTEXTOS_STORAGE)")
	autoPrune := flag.Bool("auto-prune", false, "enable automated background pruning of stale events/traces")
	index := flag.Bool("index", false, "index repository and exit")
	mcpMode := flag.Bool("mcp", false, "run MCP JSON-RPC over stdio")
	uiMode := flag.Bool("ui", false, "run web UI dashboard")
	port := flag.Int("port", 8765, "web UI dashboard port")
	flag.Parse()
	rp, _ := filepath.Abs(*repo)
	dp := *dbPath
	if dp == "" && os.Getenv("CONTEXTOS_DB") != "" {
		dp = os.Getenv("CONTEXTOS_DB")
	}
	if dp == "" {
		dp = server.DefaultDBPath()
	}
	if e := os.MkdirAll(filepath.Dir(dp), 0700); e != nil {
		dp = filepath.Join(".contextos", "context.db")
		_ = os.MkdirAll(filepath.Dir(dp), 0700)
	}
	stg := *storage
	if stg == "" && os.Getenv("CONTEXTOS_STORAGE") != "" {
		stg = os.Getenv("CONTEXTOS_STORAGE")
	}
	s, e := server.NewWithOptions(dp, rp, server.Options{
		StorageType: stg,
		AutoPrune:   *autoPrune,
	})
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	defer s.Close()
	if *index {
		if e := s.Index(); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		st, _ := s.Stats()
		fmt.Printf("indexed %v\n", st)
		return
	}
	if *mcpMode {
		if e := mcp.New(s).Run(os.Stdin, os.Stdout); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if *uiMode {
		fmt.Printf("contextd UI dashboard running on http://localhost:%d\n", *port)
		if e := ui.StartServer(s, rp, *port); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	st, _ := s.Stats()
	fmt.Printf("contextd ready: %v\n", st)
}
