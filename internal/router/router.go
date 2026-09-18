package router

import "strings"

type ModelProfile struct {
	Name            string  `json:"name"`
	InputPerM       float64 `json:"input_per_m"`
	CachedInputPerM float64 `json:"cached_input_per_m"`
	OutputPerM      float64 `json:"output_per_m"`
	ContextTokens   int     `json:"context_tokens"`
	Quality         float64 `json:"quality"`
}

var defaults = []ModelProfile{
	{Name: "gpt-5.3-codex", InputPerM: 1.75, CachedInputPerM: 0.175, OutputPerM: 7.0, ContextTokens: 400000, Quality: 0.96},
	{Name: "claude-sonnet", InputPerM: 3.0, CachedInputPerM: 0.30, OutputPerM: 15.0, ContextTokens: 200000, Quality: 0.93},
	{Name: "gemini", InputPerM: 2.0, CachedInputPerM: 0.20, OutputPerM: 8.0, ContextTokens: 1000000, Quality: 0.94},
	{Name: "local", InputPerM: 0, CachedInputPerM: 0, OutputPerM: 0, ContextTokens: 32768, Quality: 0.74},
}

func Profiles() []ModelProfile { return append([]ModelProfile(nil), defaults...) }

func Recommend(task string, budget int) ModelProfile {
	t := strings.ToLower(task)
	complexity := 0
	for _, w := range []string{"architecture", "distributed", "concurrency", "migration", "security", "performance", "deadlock", "cross-region", "refactor"} {
		if strings.Contains(t, w) {
			complexity++
		}
	}
	if complexity >= 2 {
		return defaults[0]
	}
	if budget <= 2500 {
		return defaults[3]
	}
	return defaults[1]
}
