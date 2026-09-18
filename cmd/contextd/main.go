package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"contextos/internal/mcp"
	"contextos/internal/server"
)

func main() {
	repo := flag.String("repo", ".", "repository path")
	dbPath := flag.String("db", "", "SQLite database path")
	index := flag.Bool("index", false, "index repository and exit")
	mcpMode := flag.Bool("mcp", false, "run MCP JSON-RPC over stdio")
	flag.Parse()
	rp, _ := filepath.Abs(*repo)
	dp := *dbPath
	if dp == "" {
		dp = server.DefaultDBPath()
	}
	if e := os.MkdirAll(filepath.Dir(dp), 0700); e != nil {
		dp = filepath.Join(".contextos", "context.db")
		_ = os.MkdirAll(filepath.Dir(dp), 0700)
	}
	s, e := server.New(dp, rp)
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
	st, _ := s.Stats()
	fmt.Printf("contextd ready: %v\n", st)
}
