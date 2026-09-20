package router

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDefaultProfiles(t *testing.T) {
	ResetDefaults()
	profs := Profiles()
	if len(profs) != 4 {
		t.Fatalf("expected 4 default profiles, got %d", len(profs))
	}

	p0 := profs[0]
	if p0.Name != ModelGPT53Codex {
		t.Errorf("expected %s, got %s", ModelGPT53Codex, p0.Name)
	}
	if p0.InputPerM != DefaultGPT53CodexInputPerM {
		t.Errorf("expected InputPerM %f, got %f", DefaultGPT53CodexInputPerM, p0.InputPerM)
	}
	if p0.CachedInputPerM != DefaultGPT53CodexCachedInputPerM {
		t.Errorf("expected CachedInputPerM %f, got %f", DefaultGPT53CodexCachedInputPerM, p0.CachedInputPerM)
	}
	if p0.OutputPerM != DefaultGPT53CodexOutputPerM {
		t.Errorf("expected OutputPerM %f, got %f", DefaultGPT53CodexOutputPerM, p0.OutputPerM)
	}
	if p0.ContextTokens != DefaultGPT53CodexContextTokens {
		t.Errorf("expected ContextTokens %d, got %d", DefaultGPT53CodexContextTokens, p0.ContextTokens)
	}
	if p0.Quality != DefaultGPT53CodexQuality {
		t.Errorf("expected Quality %f, got %f", DefaultGPT53CodexQuality, p0.Quality)
	}
}

func TestGetProfile(t *testing.T) {
	ResetDefaults()

	// Exact match
	p := GetProfile(ModelClaudeSonnet)
	if p.Name != ModelClaudeSonnet || p.InputPerM != DefaultClaudeSonnetInputPerM {
		t.Errorf("unexpected profile: %+v", p)
	}

	// Case-insensitive match
	pUpper := GetProfile("CLAUDE-SONNET")
	if pUpper.Name != ModelClaudeSonnet || pUpper.InputPerM != DefaultClaudeSonnetInputPerM {
		t.Errorf("unexpected case-insensitive profile: %+v", pUpper)
	}

	// Fallback for unknown model
	pUnknown := GetProfile("custom-unknown-model-xyz")
	if pUnknown.Name != "custom-unknown-model-xyz" {
		t.Errorf("expected name 'custom-unknown-model-xyz', got %s", pUnknown.Name)
	}
	if pUnknown.InputPerM != DefaultFallbackInputPerM {
		t.Errorf("expected fallback InputPerM %f, got %f", DefaultFallbackInputPerM, pUnknown.InputPerM)
	}
	if pUnknown.CachedInputPerM != DefaultFallbackCachedInputPerM {
		t.Errorf("expected fallback CachedInputPerM %f, got %f", DefaultFallbackCachedInputPerM, pUnknown.CachedInputPerM)
	}
	if pUnknown.OutputPerM != DefaultFallbackOutputPerM {
		t.Errorf("expected fallback OutputPerM %f, got %f", DefaultFallbackOutputPerM, pUnknown.OutputPerM)
	}
}

func TestLoadFromFile_Array(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "models.json")
	jsonContent := `[
		{"name": "custom-fast", "input_per_m": 0.5, "cached_input_per_m": 0.05, "output_per_m": 1.5, "context_tokens": 64000, "quality": 0.88},
		{"name": "custom-deep", "input_per_m": 5.0, "cached_input_per_m": 0.50, "output_per_m": 20.0, "context_tokens": 500000, "quality": 0.98}
	]`
	if err := os.WriteFile(cfgPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loaded, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(loaded))
	}
	if loaded[0].Name != "custom-fast" || loaded[0].InputPerM != 0.5 {
		t.Errorf("unexpected first profile: %+v", loaded[0])
	}
	if loaded[1].Name != "custom-deep" || loaded[1].OutputPerM != 20.0 {
		t.Errorf("unexpected second profile: %+v", loaded[1])
	}
}

func TestLoadFromFile_Object(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "models_obj.json")
	jsonContent := `{
		"profiles": [
			{"name": "wrapped-model", "input_per_m": 1.2, "cached_input_per_m": 0.12, "output_per_m": 4.8, "context_tokens": 128000, "quality": 0.91}
		]
	}`
	if err := os.WriteFile(cfgPath, []byte(jsonContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	loaded, err := LoadFromFile(cfgPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Name != "wrapped-model" {
		t.Fatalf("unexpected loaded profiles: %+v", loaded)
	}
}

func TestDynamicSetAndRegister(t *testing.T) {
	defer ResetDefaults()

	custom := ModelProfile{
		Name:            "llama-3.3-70b",
		InputPerM:       0.8,
		CachedInputPerM: 0.1,
		OutputPerM:      2.4,
		ContextTokens:   128000,
		Quality:         0.89,
	}

	RegisterProfile(custom)
	got := GetProfile("llama-3.3-70b")
	if got.InputPerM != 0.8 || got.Quality != 0.89 {
		t.Errorf("failed to retrieve registered profile: %+v", got)
	}

	// Update existing
	custom.InputPerM = 0.75
	RegisterProfile(custom)
	got2 := GetProfile("llama-3.3-70b")
	if got2.InputPerM != 0.75 {
		t.Errorf("failed to update registered profile: %+v", got2)
	}

	// Reset
	ResetDefaults()
	got3 := GetProfile("llama-3.3-70b")
	if got3.InputPerM != DefaultFallbackInputPerM {
		t.Errorf("expected reset to fallback, got: %+v", got3)
	}
}

func TestRecommend(t *testing.T) {
	ResetDefaults()

	// High complexity task with architecture & concurrency keywords -> GPT-5.3 Codex
	pComplex := Recommend("refactor distributed architecture for high concurrency", 10000)
	if pComplex.Name != ModelGPT53Codex {
		t.Errorf("expected %s for complex task, got %s", ModelGPT53Codex, pComplex.Name)
	}

	// Low budget <= 2500 -> Local
	pLowBudget := Recommend("fix simple typo", 1500)
	if pLowBudget.Name != ModelLocal {
		t.Errorf("expected %s for low budget, got %s", ModelLocal, pLowBudget.Name)
	}

	// Standard task -> Claude Sonnet
	pStandard := Recommend("implement user signup form validation", 8000)
	if pStandard.Name != ModelClaudeSonnet {
		t.Errorf("expected %s for standard task, got %s", ModelClaudeSonnet, pStandard.Name)
	}
}

func TestConcurrentAccess(t *testing.T) {
	defer ResetDefaults()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_ = Profiles()
		}()
		go func(id int) {
			defer wg.Done()
			_ = GetProfile(ModelClaudeSonnet)
		}(i)
		go func(id int) {
			defer wg.Done()
			RegisterProfile(ModelProfile{Name: "temp-model", InputPerM: float64(id)})
		}(i)
	}
	wg.Wait()
}
