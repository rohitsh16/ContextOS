package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"contextos/internal/mcp"
	"contextos/internal/saas/auth"
	"contextos/internal/saas/gateway"
	"contextos/internal/saas/tenant"
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
	gatewayMode := flag.Bool("gateway", false, "run multi-tenant SaaS HTTP API gateway")
	gatewayPort := flag.Int("gateway-port", 8080, "SaaS HTTP API gateway port")
	adminKey := flag.String("admin-key", "", "master admin API key for SaaS gateway (or CONTEXTOS_ADMIN_KEY)")
	tenantDataDir := flag.String("tenant-data-dir", "", "base directory for tenant storage (or CONTEXTOS_TENANT_DATA_DIR)")
	timeout := flag.Duration("timeout", 0, "query timeout (e.g. 500ms, or CONTEXTOS_TIMEOUT / CONTEXTOS_TIMEOUT_MS)")
	budget := flag.Int("budget", 4000, "default context budget in tokens (or CONTEXTOS_BUDGET)")
	minBudget := flag.Int("min-budget", 500, "minimum context budget floor under adaptive timeout (or CONTEXTOS_MIN_BUDGET)")
	adaptiveTimeout := flag.Bool("adaptive-timeout", true, "adaptively lower context budget if query approaches deadline (or CONTEXTOS_ADAPTIVE_TIMEOUT)")
	retrievalMode := flag.String("retrieval-mode", "adaptive", "retrieval mode: adaptive, indexed, or baseline (or CONTEXTOS_RETRIEVAL_MODE)")
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
		StorageType:     stg,
		AutoPrune:       *autoPrune,
		Timeout:         *timeout,
		DefaultBudget:   *budget,
		MinBudget:       *minBudget,
		AdaptiveTimeout: *adaptiveTimeout,
		RetrievalMode:   *retrievalMode,
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
		if nodes, err := s.Store.ListNodes(s.RepoID); err == nil && len(nodes) == 0 {
			_ = s.Index()
		}
		fmt.Printf("contextd UI dashboard running on http://localhost:%d\n", *port)
		if e := ui.StartServer(s, rp, *port); e != nil {
			fmt.Fprintln(os.Stderr, e)
			os.Exit(1)
		}
		return
	}
	if *gatewayMode {
		adm := *adminKey
		if adm == "" {
			adm = os.Getenv("CONTEXTOS_ADMIN_KEY")
		}
		td := *tenantDataDir
		if td == "" {
			td = os.Getenv("CONTEXTOS_TENANT_DATA_DIR")
		}
		if td == "" {
			td = filepath.Join(filepath.Dir(dp), "tenants")
		}
		authStore, err := auth.NewFileStore(filepath.Join(td, "auth.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "auth store error: %v\n", err)
			os.Exit(1)
		}
		tm, err := tenant.NewManager(tenant.Options{
			BaseDataDir:     td,
			DefaultRepoPath: rp,
			StorageType:     stg,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "tenant manager error: %v\n", err)
			os.Exit(1)
		}
		defer tm.Close()

		gw, err := gateway.New(gateway.Config{
			Port:        *gatewayPort,
			AdminAPIKey: adm,
			AuthStore:   authStore,
			TenantMgr:   tm,
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "gateway init error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("ContextOS SaaS Gateway running on http://localhost:%d\n", *gatewayPort)
		srv := &http.Server{
			Addr:    fmt.Sprintf(":%d", *gatewayPort),
			Handler: gw.Handler(),
		}
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "gateway error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	st, _ := s.Stats()
	fmt.Printf("contextd ready: %v\n", st)
}
