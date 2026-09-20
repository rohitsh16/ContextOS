package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"contextos/internal/textutil"
)

var (
	ErrDimensionMismatch = errors.New("vector dimension mismatch")
	ErrEmptyInput        = errors.New("empty text input")
	ErrProviderFailed    = errors.New("embedding provider failure")
)

// EmbeddingProvider abstracts dense embedding generation across offline and remote providers.
type EmbeddingProvider interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	Dimension() int
}

// CosineSimilarity computes cosine similarity between two float32 vectors.
// Returns ErrDimensionMismatch if dimensions do not match.
func CosineSimilarity(a, b []float32) (float32, error) {
	if len(a) != len(b) {
		return 0, fmt.Errorf("%w: len(a)=%d, len(b)=%d", ErrDimensionMismatch, len(a), len(b))
	}
	if len(a) == 0 {
		return 0, nil
	}
	var dot, na, nb float64
	for i := 0; i < len(a); i++ {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0, nil
	}
	sim := dot / (math.Sqrt(na) * math.Sqrt(nb))
	if sim > 1.0 {
		sim = 1.0
	} else if sim < -1.0 {
		sim = -1.0
	}
	return float32(sim), nil
}

// ─── HashEmbeddingProvider (Zero-Dependency Baseline) ─────────────────────────

// HashEmbeddingProvider produces 128-dimensional normalized feature-hash vectors
// with zero external dependencies.
type HashEmbeddingProvider struct {
	dims int
}

func NewHashEmbeddingProvider() *HashEmbeddingProvider {
	return &HashEmbeddingProvider{dims: 128}
}

func (h *HashEmbeddingProvider) Dimension() int {
	return h.dims
}

func (h *HashEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	results := make([][]float32, len(texts))
	for i, t := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		results[i] = computeHashVector(t, h.dims)
	}
	return results, nil
}

func computeHashVector(s string, dims int) []float32 {
	v := make([]float64, dims)
	toks := textutil.Tokens(s)
	for _, t := range toks {
		h := fnv.New32a()
		_, _ = h.Write([]byte(t))
		idx := int(h.Sum32() % uint32(dims))
		v[idx] += 1.0
		if len(t) >= 3 {
			for i := 0; i+3 <= len(t); i++ {
				h2 := fnv.New32a()
				_, _ = h2.Write([]byte(t[i : i+3]))
				idx2 := int(h2.Sum32() % uint32(dims))
				v[idx2] += 0.35
			}
		}
	}
	var norm float64
	for i := 0; i < dims; i++ {
		norm += v[i] * v[i]
	}
	out := make([]float32, dims)
	if norm > 0 {
		sqrtNorm := math.Sqrt(norm)
		for i := 0; i < dims; i++ {
			out[i] = float32(v[i] / sqrtNorm)
		}
	}
	return out
}

// ─── LocalEmbeddingProvider (Deterministic Offline Semantic Model) ────────────

// LocalEmbeddingProvider provides deterministic local dense embeddings using
// subword n-gram hashing and multi-head frequency projection.
type LocalEmbeddingProvider struct {
	dims int
}

func NewLocalEmbeddingProvider(dims int) *LocalEmbeddingProvider {
	if dims <= 0 {
		dims = 256
	}
	return &LocalEmbeddingProvider{dims: dims}
}

func (l *LocalEmbeddingProvider) Dimension() int {
	return l.dims
}

func (l *LocalEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	res := make([][]float32, len(texts))
	for i, t := range texts {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		res[i] = l.embedOne(t)
	}
	return res, nil
}

func (l *LocalEmbeddingProvider) embedOne(text string) []float32 {
	v := make([]float64, l.dims)
	raw := strings.ToLower(text)
	words := regexp.MustCompile(`[^a-z0-9_]+`).ReplaceAllString(raw, " ")
	tokens := strings.Fields(words)

	for pos, tok := range tokens {
		posWeight := 1.0 / (1.0 + 0.05*float64(pos))
		// Word unigram hash
		h := fnv.New64a()
		_, _ = h.Write([]byte(tok))
		u := h.Sum64()
		idx1 := int(u % uint64(l.dims))
		sign1 := 1.0
		if (u >> 32)&1 == 1 {
			sign1 = -1.0
		}
		v[idx1] += sign1 * 1.5 * posWeight

		// Character 3-grams & 4-grams for subword morphological capture
		tokRunes := []rune(tok)
		for n := 3; n <= 4; n++ {
			for i := 0; i+n <= len(tokRunes); i++ {
				ngram := string(tokRunes[i : i+n])
				hn := fnv.New64a()
				_, _ = hn.Write([]byte(ngram))
				un := hn.Sum64()
				idxN := int(un % uint64(l.dims))
				signN := 1.0
				if (un >> 32)&1 == 1 {
					signN = -1.0
				}
				v[idxN] += signN * 0.4 * posWeight
			}
		}
	}

	var norm float64
	for i := 0; i < l.dims; i++ {
		norm += v[i] * v[i]
	}
	out := make([]float32, l.dims)
	if norm > 0 {
		sqrtNorm := math.Sqrt(norm)
		for i := 0; i < l.dims; i++ {
			out[i] = float32(v[i] / sqrtNorm)
		}
	}
	return out
}

// ─── HTTPEmbeddingProvider (Remote API Endpoint) ───────────────────────────────

type HTTPEmbeddingProvider struct {
	Endpoint   string
	APIKey     string
	Model      string
	Dims       int
	Client     *http.Client
	MaxRetries int
}

func NewHTTPEmbeddingProvider(endpoint, apiKey, model string, dims int) *HTTPEmbeddingProvider {
	if dims <= 0 {
		dims = 1536
	}
	return &HTTPEmbeddingProvider{
		Endpoint:   endpoint,
		APIKey:     apiKey,
		Model:      model,
		Dims:       dims,
		Client:     &http.Client{Timeout: 10 * time.Second},
		MaxRetries: 2,
	}
}

func (h *HTTPEmbeddingProvider) Dimension() int {
	return h.Dims
}

type openAIEmbeddingRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (h *HTTPEmbeddingProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	if h.Endpoint == "" {
		return nil, fmt.Errorf("%w: empty endpoint", ErrProviderFailed)
	}

	reqBody := openAIEmbeddingRequest{
		Input: texts,
		Model: h.Model,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp *http.Response
	for attempt := 0; attempt <= h.MaxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, h.Endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if h.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+h.APIKey)
		}

		resp, err = h.Client.Do(httpReq)
		if err == nil && resp.StatusCode == http.StatusOK {
			break
		}
		if resp != nil {
			resp.Body.Close()
		}
		if attempt == h.MaxRetries {
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrProviderFailed, err)
			}
			return nil, fmt.Errorf("%w: HTTP %d", ErrProviderFailed, resp.StatusCode)
		}
		time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
	}

	defer resp.Body.Close()
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var parsed openAIEmbeddingResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if parsed.Error != nil {
		return nil, fmt.Errorf("%w: %s", ErrProviderFailed, parsed.Error.Message)
	}

	if len(parsed.Data) != len(texts) {
		return nil, fmt.Errorf("%w: expected %d embeddings, received %d", ErrProviderFailed, len(texts), len(parsed.Data))
	}

	// Sort by index
	sort.Slice(parsed.Data, func(i, j int) bool {
		return parsed.Data[i].Index < parsed.Data[j].Index
	})

	out := make([][]float32, len(texts))
	for i, item := range parsed.Data {
		if len(item.Embedding) != h.Dims && h.Dims > 0 {
			// update detected dims if not strictly matching
			h.Dims = len(item.Embedding)
		}
		out[i] = item.Embedding
	}
	return out, nil
}

// ─── Late Interaction (ColBERT-style) ─────────────────────────────────────────

// LateInteractionScore computes token-level MaxSim between query token embeddings and doc token embeddings:
// Score(Q, D) = sum_{q_i in Q} max_{d_j in D} sim(q_i, d_j)
func LateInteractionScore(qEmbeds, docEmbeds [][]float32) (float32, error) {
	if len(qEmbeds) == 0 || len(docEmbeds) == 0 {
		return 0, nil
	}
	var totalScore float32
	for _, qVec := range qEmbeds {
		maxSim := float32(-1.0)
		for _, dVec := range docEmbeds {
			sim, err := CosineSimilarity(qVec, dVec)
			if err != nil {
				return 0, err
			}
			if sim > maxSim {
				maxSim = sim
			}
		}
		if maxSim > 0 {
			totalScore += maxSim
		}
	}
	return totalScore, nil
}

// ─── Hybrid Retrieval Tier & RRF Fusion ───────────────────────────────────────

type ScoredCandidate struct {
	ID    string
	Score float64
	Rank  int
}

// HybridRanker combines Lexical (BM25), Graph Centrality (PPR), and Dense Semantic signals.
type HybridRanker struct {
	K float64 // RRF constant, default 60.0
}

func NewHybridRanker(k float64) *HybridRanker {
	if k <= 0 {
		k = 60.0
	}
	return &HybridRanker{K: k}
}

// FuseScores computes Reciprocal Rank Fusion across lexical, graph, and dense ranking lists.
// If dense rankings are empty (e.g. provider failure), it fuses lexical and graph gracefully.
func (hr *HybridRanker) FuseScores(
	candidateIDs []string,
	lexicalScores map[string]float64,
	graphScores map[string]float64,
	denseScores map[string]float64,
	wL, wG, wD float64,
) map[string]float64 {
	if wL <= 0 && wG <= 0 && wD <= 0 {
		wL, wG, wD = 1.0, 1.0, 1.0
	}

	// Build ranks for each signal
	rankList := func(scores map[string]float64) map[string]int {
		type kv struct {
			id    string
			score float64
		}
		var list []kv
		for _, id := range candidateIDs {
			list = append(list, kv{id: id, score: scores[id]})
		}
		sort.Slice(list, func(i, j int) bool {
			return list[i].score > list[j].score
		})
		ranks := make(map[string]int, len(list))
		for r, item := range list {
			ranks[item.id] = r + 1
		}
		return ranks
	}

	lexRanks := rankList(lexicalScores)
	graphRanks := rankList(graphScores)

	hasDense := len(denseScores) > 0
	var denseRanks map[string]int
	if hasDense {
		denseRanks = rankList(denseScores)
	}

	fused := make(map[string]float64, len(candidateIDs))
	for _, id := range candidateIDs {
		rrf := 0.0
		if r, ok := lexRanks[id]; ok && lexicalScores[id] > 0 {
			rrf += wL / (hr.K + float64(r))
		}
		if r, ok := graphRanks[id]; ok && graphScores[id] > 0 {
			rrf += wG / (hr.K + float64(r))
		}
		if hasDense {
			if r, ok := denseRanks[id]; ok && denseScores[id] > 0 {
				rrf += wD / (hr.K + float64(r))
			}
		}
		fused[id] = rrf
	}
	return fused
}
