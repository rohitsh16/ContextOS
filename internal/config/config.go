package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config represents logical, machine-independent project configuration.
type Config struct {
	Version      int                `json:"version"`
	Project      ProjectConfig      `json:"project"`
	Index        IndexConfig        `json:"index"`
	Memory       MemoryConfig       `json:"memory"`
	Retrieval    RetrievalConfig    `json:"retrieval"`
	Allocator    AllocatorConfig    `json:"allocator"`
	Cache        CacheConfig        `json:"cache"`
	Integrations IntegrationsConfig `json:"integrations"`
}

type ProjectConfig struct {
	Name string `json:"name"`
}

type IndexConfig struct {
	Enabled    bool `json:"enabled"`
	Background bool `json:"background"`
}

type MemoryConfig struct {
	Enabled bool `json:"enabled"`
}

type RetrievalConfig struct {
	Lexical         bool   `json:"lexical"`
	Semantic        bool   `json:"semantic"`
	Graph           bool   `json:"graph"`
	TimeoutMs       int    `json:"timeout_ms"`
	AdaptiveTimeout bool   `json:"adaptive_timeout"`
	Mode            string `json:"mode"`
}

type AllocatorConfig struct {
	BudgetMode      string `json:"budget_mode"`
	DefaultBudget   int    `json:"default_budget"`
	MinBudgetTokens int    `json:"min_budget_tokens"`
}

type CacheConfig struct {
	Enabled      bool   `json:"enabled"`
	StablePrefix string `json:"stable_prefix"`
}

type IntegrationsConfig struct {
	AutoCapture bool `json:"auto_capture"`
}

// Default returns standard logical defaults.
func Default() Config {
	return Config{
		Version: 1,
		Project: ProjectConfig{
			Name: "contextos-project",
		},
		Index: IndexConfig{
			Enabled:    true,
			Background: true,
		},
		Memory: MemoryConfig{
			Enabled: true,
		},
		Retrieval: RetrievalConfig{
			Lexical:  true,
			Semantic: true,
			Graph:    true,
		},
		Allocator: AllocatorConfig{
			BudgetMode: "adaptive",
		},
		Cache: CacheConfig{
			Enabled:      true,
			StablePrefix: "adaptive",
		},
		Integrations: IntegrationsConfig{
			AutoCapture: true,
		},
	}
}

// ConfigPath returns the standard path to .contextos/config.toml.
func ConfigPath(repoRoot string) string {
	return filepath.Join(repoRoot, ".contextos", "config.toml")
}

// Load reads and parses .contextos/config.toml, falling back to Default() if absent.
func Load(repoRoot string) (Config, error) {
	cfg := Default()
	p := ConfigPath(repoRoot)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	currentSection := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		v = strings.Trim(v, `"'`)

		switch currentSection {
		case "":
			if k == "version" {
				cfg.Version, _ = strconv.Atoi(v)
			}
		case "project":
			if k == "name" {
				cfg.Project.Name = v
			}
		case "index":
			if k == "enabled" {
				cfg.Index.Enabled = (v == "true")
			} else if k == "background" {
				cfg.Index.Background = (v == "true")
			}
		case "memory":
			if k == "enabled" {
				cfg.Memory.Enabled = (v == "true")
			}
		case "retrieval":
			if k == "lexical" {
				cfg.Retrieval.Lexical = (v == "true")
			} else if k == "semantic" {
				cfg.Retrieval.Semantic = (v == "true")
			} else if k == "graph" {
				cfg.Retrieval.Graph = (v == "true")
			} else if k == "timeout_ms" {
				cfg.Retrieval.TimeoutMs, _ = strconv.Atoi(v)
			} else if k == "adaptive_timeout" {
				cfg.Retrieval.AdaptiveTimeout = (v == "true")
			} else if k == "mode" {
				cfg.Retrieval.Mode = v
			}
		case "allocator":
			if k == "budget_mode" {
				cfg.Allocator.BudgetMode = v
			} else if k == "default_budget" {
				cfg.Allocator.DefaultBudget, _ = strconv.Atoi(v)
			} else if k == "min_budget_tokens" {
				cfg.Allocator.MinBudgetTokens, _ = strconv.Atoi(v)
			}
		case "cache":
			if k == "enabled" {
				cfg.Cache.Enabled = (v == "true")
			} else if k == "stable_prefix" {
				cfg.Cache.StablePrefix = v
			}
		case "integrations":
			if k == "auto_capture" {
				cfg.Integrations.AutoCapture = (v == "true")
			}
		}
	}

	return cfg, scanner.Err()
}

// Save serializes the configuration to .contextos/config.toml.
func Save(repoRoot string, cfg Config) error {
	p := ConfigPath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		return err
	}

	content := fmt.Sprintf(`version = %d

[project]
name = "%s"

[index]
enabled = %t
background = %t

[memory]
enabled = %t

[retrieval]
lexical = %t
semantic = %t
graph = %t

[allocator]
budget_mode = "%s"

[cache]
enabled = %t
stable_prefix = "%s"

[integrations]
auto_capture = %t
`,
		cfg.Version,
		cfg.Project.Name,
		cfg.Index.Enabled,
		cfg.Index.Background,
		cfg.Memory.Enabled,
		cfg.Retrieval.Lexical,
		cfg.Retrieval.Semantic,
		cfg.Retrieval.Graph,
		cfg.Allocator.BudgetMode,
		cfg.Cache.Enabled,
		cfg.Cache.StablePrefix,
		cfg.Integrations.AutoCapture,
	)

	return os.WriteFile(p, []byte(content), 0644)
}
