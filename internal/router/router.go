package router

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Standard Model Identifiers
const (
	ModelGPT53Codex   = "gpt-5.3-codex"
	ModelClaudeSonnet  = "claude-sonnet"
	ModelGemini        = "gemini"
	ModelLocal         = "local"
	ModelFallback      = "generic"
)

// Default Model Pricing, Context Tokens, and Quality Ratings
const (
	// GPT-5.3 Codex
	DefaultGPT53CodexInputPerM       = 1.75
	DefaultGPT53CodexCachedInputPerM = 0.175
	DefaultGPT53CodexOutputPerM      = 7.0
	DefaultGPT53CodexContextTokens   = 400000
	DefaultGPT53CodexQuality         = 0.96

	// Claude Sonnet
	DefaultClaudeSonnetInputPerM       = 3.0
	DefaultClaudeSonnetCachedInputPerM = 0.30
	DefaultClaudeSonnetOutputPerM      = 15.0
	DefaultClaudeSonnetContextTokens   = 200000
	DefaultClaudeSonnetQuality         = 0.93

	// Gemini
	DefaultGeminiInputPerM       = 2.0
	DefaultGeminiCachedInputPerM = 0.20
	DefaultGeminiOutputPerM      = 8.0
	DefaultGeminiContextTokens   = 1000000
	DefaultGeminiQuality         = 0.94

	// Local
	DefaultLocalInputPerM       = 0.0
	DefaultLocalCachedInputPerM = 0.0
	DefaultLocalOutputPerM      = 0.0
	DefaultLocalContextTokens   = 32768
	DefaultLocalQuality         = 0.74

	// Fallback / Unknown Model Default Pricing
	DefaultFallbackInputPerM       = 1.50
	DefaultFallbackCachedInputPerM = 0.15
	DefaultFallbackOutputPerM      = 6.00
	DefaultFallbackContextTokens   = 128000
	DefaultFallbackQuality         = 0.85

	// Configuration Discovery Constants
	EnvConfigPath   = "CONTEXTOS_MODELS_CONFIG"
	DefaultFileName = "models.json"
)

// ModelProfile represents pricing and capacity for a model.
type ModelProfile struct {
	Name            string  `json:"name"`
	InputPerM       float64 `json:"input_per_m"`
	CachedInputPerM float64 `json:"cached_input_per_m"`
	OutputPerM      float64 `json:"output_per_m"`
	ContextTokens   int     `json:"context_tokens"`
	Quality         float64 `json:"quality"`
}

// ConfigWrapper supports both direct array and object formats in JSON files.
type ConfigWrapper struct {
	Profiles []ModelProfile `json:"profiles"`
}

// DefaultProfiles holds the built-in constant profiles.
var DefaultProfiles = []ModelProfile{
	{
		Name:            ModelGPT53Codex,
		InputPerM:       DefaultGPT53CodexInputPerM,
		CachedInputPerM: DefaultGPT53CodexCachedInputPerM,
		OutputPerM:      DefaultGPT53CodexOutputPerM,
		ContextTokens:   DefaultGPT53CodexContextTokens,
		Quality:         DefaultGPT53CodexQuality,
	},
	{
		Name:            ModelClaudeSonnet,
		InputPerM:       DefaultClaudeSonnetInputPerM,
		CachedInputPerM: DefaultClaudeSonnetCachedInputPerM,
		OutputPerM:      DefaultClaudeSonnetOutputPerM,
		ContextTokens:   DefaultClaudeSonnetContextTokens,
		Quality:         DefaultClaudeSonnetQuality,
	},
	{
		Name:            ModelGemini,
		InputPerM:       DefaultGeminiInputPerM,
		CachedInputPerM: DefaultGeminiCachedInputPerM,
		OutputPerM:      DefaultGeminiOutputPerM,
		ContextTokens:   DefaultGeminiContextTokens,
		Quality:         DefaultGeminiQuality,
	},
	{
		Name:            ModelLocal,
		InputPerM:       DefaultLocalInputPerM,
		CachedInputPerM: DefaultLocalCachedInputPerM,
		OutputPerM:      DefaultLocalOutputPerM,
		ContextTokens:   DefaultLocalContextTokens,
		Quality:         DefaultLocalQuality,
	},
}

// FallbackProfile returns the baseline profile for unrecognized models.
func FallbackProfile() ModelProfile {
	return ModelProfile{
		Name:            ModelFallback,
		InputPerM:       DefaultFallbackInputPerM,
		CachedInputPerM: DefaultFallbackCachedInputPerM,
		OutputPerM:      DefaultFallbackOutputPerM,
		ContextTokens:   DefaultFallbackContextTokens,
		Quality:         DefaultFallbackQuality,
	}
}

var (
	mu             sync.RWMutex
	activeProfiles []ModelProfile
)

func init() {
	InitFromConfig()
}

// DefaultConfigPaths returns candidate file paths for models.json in order of precedence.
func DefaultConfigPaths() []string {
	var paths []string
	if envPath := os.Getenv(EnvConfigPath); envPath != "" {
		paths = append(paths, envPath)
	}
	if cwd, err := os.Getwd(); err == nil {
		paths = append(paths, filepath.Join(cwd, ".contextos", DefaultFileName))
		paths = append(paths, filepath.Join(cwd, DefaultFileName))
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".contextos", DefaultFileName))
	}
	return paths
}

// LoadFromFile reads and parses a models.json file. It accepts either an array of profiles
// or an object with a "profiles" key.
func LoadFromFile(path string) ([]ModelProfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Try parsing as array
	var list []ModelProfile
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	// Try parsing as object wrapper
	var wrapper ConfigWrapper
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Profiles) > 0 {
		return wrapper.Profiles, nil
	}
	return nil, fmt.Errorf("invalid or empty model profile config in %s", path)
}

// InitFromConfig initializes active profiles from the first available configuration file,
// or falls back to built-in DefaultProfiles if none exist.
func InitFromConfig() {
	mu.Lock()
	defer mu.Unlock()

	for _, p := range DefaultConfigPaths() {
		if loaded, err := LoadFromFile(p); err == nil && len(loaded) > 0 {
			activeProfiles = loaded
			return
		}
	}
	activeProfiles = append([]ModelProfile(nil), DefaultProfiles...)
}

// Profiles returns a defensive copy of currently active model profiles.
func Profiles() []ModelProfile {
	mu.RLock()
	defer mu.RUnlock()
	if len(activeProfiles) == 0 {
		return append([]ModelProfile(nil), DefaultProfiles...)
	}
	return append([]ModelProfile(nil), activeProfiles...)
}

// GetProfile returns the profile for a given model name (case-insensitive).
// If not found, it returns FallbackProfile().
func GetProfile(name string) ModelProfile {
	profs := Profiles()
	for _, p := range profs {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	fb := FallbackProfile()
	if name != "" {
		fb.Name = name
	}
	return fb
}

// SetProfiles updates the active model profiles in memory.
func SetProfiles(profiles []ModelProfile) {
	mu.Lock()
	defer mu.Unlock()
	if len(profiles) == 0 {
		activeProfiles = append([]ModelProfile(nil), DefaultProfiles...)
		return
	}
	activeProfiles = append([]ModelProfile(nil), profiles...)
}

// RegisterProfile adds or updates a specific model profile in the active set.
func RegisterProfile(p ModelProfile) {
	mu.Lock()
	defer mu.Unlock()
	for i, existing := range activeProfiles {
		if strings.EqualFold(existing.Name, p.Name) {
			activeProfiles[i] = p
			return
		}
	}
	activeProfiles = append(activeProfiles, p)
}

// ResetDefaults restores active profiles to the built-in constant defaults.
func ResetDefaults() {
	mu.Lock()
	defer mu.Unlock()
	activeProfiles = append([]ModelProfile(nil), DefaultProfiles...)
}

// Recommend returns the best suited model profile based on task complexity and token budget.
func Recommend(task string, budget int) ModelProfile {
	profs := Profiles()
	if len(profs) == 0 {
		return FallbackProfile()
	}

	t := strings.ToLower(task)
	complexity := 0
	for _, w := range []string{"architecture", "distributed", "concurrency", "migration", "security", "performance", "deadlock", "cross-region", "refactor"} {
		if strings.Contains(t, w) {
			complexity++
		}
	}

	// High complexity: favor codex
	if complexity >= 2 {
		for _, p := range profs {
			if strings.EqualFold(p.Name, ModelGPT53Codex) {
				return p
			}
		}
		return profs[0]
	}

	// Tight budget: favor local / lightweight
	if budget <= 2500 {
		for _, p := range profs {
			if strings.EqualFold(p.Name, ModelLocal) {
				return p
			}
		}
		return profs[len(profs)-1]
	}

	// Default balanced: favor Claude Sonnet
	for _, p := range profs {
		if strings.EqualFold(p.Name, ModelClaudeSonnet) {
			return p
		}
	}
	return profs[0]
}
