package semantic

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestCosineSimilarity(t *testing.T) {
	v1 := []float32{1.0, 0.0, 0.0}
	v2 := []float32{1.0, 0.0, 0.0}
	v3 := []float32{0.0, 1.0, 0.0}

	sim1, err := CosineSimilarity(v1, v2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(float64(sim1)-1.0) > 1e-4 {
		t.Errorf("identical vectors want 1.0, got %.4f", sim1)
	}

	simOrth, err := CosineSimilarity(v1, v3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if math.Abs(float64(simOrth)-0.0) > 1e-4 {
		t.Errorf("orthogonal vectors want 0.0, got %.4f", simOrth)
	}
}

func TestCosineSimilarityDimensionMismatch(t *testing.T) {
	v1 := []float32{1.0, 2.0}
	v2 := []float32{1.0, 2.0, 3.0}
	_, err := CosineSimilarity(v1, v2)
	if !errors.Is(err, ErrDimensionMismatch) {
		t.Errorf("expected ErrDimensionMismatch, got %v", err)
	}
}

func TestHashEmbeddingProvider(t *testing.T) {
	ctx := context.Background()
	provider := NewHashEmbeddingProvider()

	if provider.Dimension() != 128 {
		t.Errorf("expected 128 dimensions, got %d", provider.Dimension())
	}

	texts := []string{
		"kafka transaction outbox",
		"kafka retries error handling",
		"completely unrelated documentation",
	}

	embeds, err := provider.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(embeds) != 3 {
		t.Fatalf("expected 3 embeddings, got %d", len(embeds))
	}

	// Related texts should have higher cosine similarity than unrelated texts
	simRelated, err := CosineSimilarity(embeds[0], embeds[1])
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	simUnrelated, err := CosineSimilarity(embeds[0], embeds[2])
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}

	if simRelated <= simUnrelated {
		t.Errorf("expected related similarity (%.4f) > unrelated similarity (%.4f)", simRelated, simUnrelated)
	}
}

func TestLocalEmbeddingProviderBatchVsSingle(t *testing.T) {
	ctx := context.Background()
	provider := NewLocalEmbeddingProvider(256)

	texts := []string{
		"context planning with token budget",
		"personalized pagerank graph centrality",
	}

	batchEmbeds, err := provider.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("batch Embed failed: %v", err)
	}

	single0, err := provider.Embed(ctx, []string{texts[0]})
	if err != nil {
		t.Fatalf("single Embed 0 failed: %v", err)
	}
	single1, err := provider.Embed(ctx, []string{texts[1]})
	if err != nil {
		t.Fatalf("single Embed 1 failed: %v", err)
	}

	// Compare batch vs single
	for i := 0; i < provider.Dimension(); i++ {
		if math.Abs(float64(batchEmbeds[0][i]-single0[0][i])) > 1e-6 {
			t.Errorf("dim %d mismatch between batch and single for text 0", i)
		}
		if math.Abs(float64(batchEmbeds[1][i]-single1[0][i])) > 1e-6 {
			t.Errorf("dim %d mismatch between batch and single for text 1", i)
		}
	}
}

func TestDuplicateEmbedding(t *testing.T) {
	ctx := context.Background()
	provider := NewLocalEmbeddingProvider(128)

	texts := []string{
		"identical content text",
		"identical content text",
	}
	embeds, err := provider.Embed(ctx, texts)
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	sim, err := CosineSimilarity(embeds[0], embeds[1])
	if err != nil {
		t.Fatalf("CosineSimilarity failed: %v", err)
	}
	if math.Abs(float64(sim)-1.0) > 1e-4 {
		t.Errorf("duplicate texts must have similarity ~1.0, got %.4f", sim)
	}
}

func TestEmptyEmbeddingInput(t *testing.T) {
	ctx := context.Background()
	provider := NewLocalEmbeddingProvider(128)

	embeds, err := provider.Embed(ctx, []string{})
	if err != nil {
		t.Fatalf("Embed with empty slice failed: %v", err)
	}
	if len(embeds) != 0 {
		t.Errorf("expected 0 embeddings, got %d", len(embeds))
	}
}

func TestLateInteractionColBERT(t *testing.T) {
	// 2 query tokens, 3 doc tokens
	qTokens := [][]float32{
		{1.0, 0.0, 0.0}, // matches docToken 0
		{0.0, 1.0, 0.0}, // matches docToken 1
	}
	docTokens := [][]float32{
		{0.9, 0.1, 0.0},
		{0.0, 0.8, 0.2},
		{0.0, 0.0, 1.0},
	}

	score, err := LateInteractionScore(qTokens, docTokens)
	if err != nil {
		t.Fatalf("LateInteractionScore failed: %v", err)
	}

	// MaxSim(q0, doc) ~ 0.9939
	// MaxSim(q1, doc) ~ 0.9701
	// Total score ~ 1.964
	if score < 1.8 {
		t.Errorf("expected high late interaction match, got %.4f", score)
	}
}

func TestHybridFusionAndProviderFailureFallback(t *testing.T) {
	ranker := NewHybridRanker(60.0)

	candidates := []string{"doc1", "doc2", "doc3"}
	lexicalScores := map[string]float64{
		"doc1": 2.5,
		"doc2": 1.5,
		"doc3": 0.0,
	}
	graphScores := map[string]float64{
		"doc1": 0.8,
		"doc2": 0.4,
		"doc3": 0.2,
	}

	// Scenario 1: Full hybrid with dense scores
	denseScores := map[string]float64{
		"doc1": 0.9,
		"doc2": 0.7,
		"doc3": 0.1,
	}
	fused := ranker.FuseScores(candidates, lexicalScores, graphScores, denseScores, 1.0, 1.0, 1.0)
	if fused["doc1"] <= fused["doc2"] {
		t.Errorf("doc1 should rank highest in hybrid, got doc1=%.4f, doc2=%.4f", fused["doc1"], fused["doc2"])
	}

	// Scenario 2: Provider failure fallback (denseScores is empty)
	fusedFallback := ranker.FuseScores(candidates, lexicalScores, graphScores, map[string]float64{}, 1.0, 1.0, 1.0)
	if fusedFallback["doc1"] <= fusedFallback["doc2"] {
		t.Errorf("doc1 should still rank highest in fallback, got doc1=%.4f, doc2=%.4f", fusedFallback["doc1"], fusedFallback["doc2"])
	}
	if fusedFallback["doc3"] >= fusedFallback["doc2"] {
		t.Errorf("doc2 should rank higher than doc3 in fallback, got doc2=%.4f, doc3=%.4f", fusedFallback["doc2"], fusedFallback["doc3"])
	}
}
