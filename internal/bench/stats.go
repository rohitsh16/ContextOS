package bench

import (
	"math"
	"math/rand"
	"sort"
)

// SummaryStats holds descriptive statistics for a distribution of metric values.
type SummaryStats struct {
	Count  int     `json:"count"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	StdDev float64 `json:"std_dev"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
}

// ComputeSummary computes descriptive statistics over a slice of float64.
func ComputeSummary(xs []float64) SummaryStats {
	if len(xs) == 0 {
		return SummaryStats{}
	}
	sorted := make([]float64, len(xs))
	copy(sorted, xs)
	sort.Float64s(sorted)

	sum := 0.0
	for _, x := range sorted {
		sum += x
	}
	mean := sum / float64(len(sorted))

	var median float64
	n := len(sorted)
	if n%2 == 0 {
		median = (sorted[n/2-1] + sorted[n/2]) / 2.0
	} else {
		median = sorted[n/2]
	}

	var varianceSum float64
	for _, x := range sorted {
		diff := x - mean
		varianceSum += diff * diff
	}
	variance := 0.0
	if n > 1 {
		variance = varianceSum / float64(n-1)
	}

	return SummaryStats{
		Count:  n,
		Mean:   mean,
		Median: median,
		StdDev: math.Sqrt(variance),
		Min:    sorted[0],
		Max:    sorted[n-1],
	}
}

// PairedDelta computes element-wise differences: Δ_i = new_i - baseline_i.
func PairedDelta(candidate, baseline []float64) []float64 {
	n := len(candidate)
	if len(baseline) < n {
		n = len(baseline)
	}
	deltas := make([]float64, n)
	for i := 0; i < n; i++ {
		deltas[i] = candidate[i] - baseline[i]
	}
	return deltas
}

// ConfidenceInterval represents a lower and upper bound at a given confidence level.
type ConfidenceInterval struct {
	Lower      float64 `json:"lower"`
	Upper      float64 `json:"upper"`
	Confidence float64 `json:"confidence"`
}

// BootstrapCI computes an empirical bootstrap confidence interval for the mean
// of a sample using nResamples iterations.
func BootstrapCI(deltas []float64, nResamples int, confidence float64, seed int64) ConfidenceInterval {
	if len(deltas) == 0 {
		return ConfidenceInterval{Confidence: confidence}
	}
	if nResamples <= 0 {
		nResamples = 1000
	}
	r := rand.New(rand.NewSource(seed))
	n := len(deltas)
	means := make([]float64, nResamples)

	for b := 0; b < nResamples; b++ {
		sum := 0.0
		for i := 0; i < n; i++ {
			idx := r.Intn(n)
			sum += deltas[idx]
		}
		means[b] = sum / float64(n)
	}

	sort.Float64s(means)
	alpha := 1.0 - confidence
	lowerIdx := int(math.Floor(alpha / 2.0 * float64(nResamples)))
	upperIdx := int(math.Ceil((1.0 - alpha/2.0) * float64(nResamples)))
	if lowerIdx < 0 {
		lowerIdx = 0
	}
	if upperIdx >= nResamples {
		upperIdx = nResamples - 1
	}

	return ConfidenceInterval{
		Lower:      means[lowerIdx],
		Upper:      means[upperIdx],
		Confidence: confidence,
	}
}

// PairedPermutationTest computes a two-sided p-value for the hypothesis that the
// paired mean difference is zero by randomly flipping the signs of paired differences.
func PairedPermutationTest(deltas []float64, nPermutations int, seed int64) float64 {
	n := len(deltas)
	if n == 0 {
		return 1.0
	}
	if nPermutations <= 0 {
		nPermutations = 5000
	}

	obsSum := 0.0
	for _, d := range deltas {
		obsSum += d
	}
	obsMean := math.Abs(obsSum / float64(n))

	r := rand.New(rand.NewSource(seed))
	countExtreme := 0

	for p := 0; p < nPermutations; p++ {
		permSum := 0.0
		for i := 0; i < n; i++ {
			if r.Intn(2) == 1 {
				permSum += deltas[i]
			} else {
				permSum -= deltas[i]
			}
		}
		permMean := math.Abs(permSum / float64(n))
		if permMean >= obsMean-1e-12 {
			countExtreme++
		}
	}

	return float64(countExtreme) / float64(nPermutations)
}

// CliffsDelta computes Cliff's delta effect size for paired observations:
// d = (#(Δ > 0) - #(Δ < 0)) / n.
// Returns effect size d in [-1, 1] and qualitative interpretation.
func CliffsDelta(deltas []float64) (float64, string) {
	if len(deltas) == 0 {
		return 0.0, "negligible"
	}
	pos, neg := 0, 0
	for _, d := range deltas {
		if d > 1e-9 {
			pos++
		} else if d < -1e-9 {
			neg++
		}
	}
	d := float64(pos-neg) / float64(len(deltas))

	absD := math.Abs(d)
	var interp string
	switch {
	case absD < 0.147:
		interp = "negligible"
	case absD < 0.33:
		interp = "small"
	case absD < 0.474:
		interp = "medium"
	default:
		interp = "large"
	}
	return d, interp
}

// HypothesisResult stores the outcome of a multiple-comparison test after Holm-Bonferroni correction.
type HypothesisResult struct {
	Name            string  `json:"name"`
	RawPValue       float64 `json:"raw_p_value"`
	Rank            int     `json:"rank"`
	AdjustedAlpha   float64 `json:"adjusted_alpha"`
	SignificantDiff bool    `json:"significant_diff"`
}

// HolmBonferroni applies the Holm-Bonferroni step-down procedure to control the
// family-wise error rate across m hypothesis tests at significance level alpha.
func HolmBonferroni(comparisons map[string]float64, alpha float64) []HypothesisResult {
	type kv struct {
		name string
		p    float64
	}
	var pairs []kv
	for k, v := range comparisons {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].p < pairs[j].p
	})

	m := len(pairs)
	results := make([]HypothesisResult, m)
	stopped := false

	for i, pair := range pairs {
		rank := i + 1
		thresh := alpha / float64(m-rank+1)
		sig := false
		if !stopped {
			if pair.p <= thresh {
				sig = true
			} else {
				stopped = true
			}
		}
		results[i] = HypothesisResult{
			Name:            pair.name,
			RawPValue:       pair.p,
			Rank:            rank,
			AdjustedAlpha:   thresh,
			SignificantDiff: sig,
		}
	}
	return results
}

// WilsonScoreInterval calculates the Wilson score interval for a binomial proportion k/n
// at the specified confidence level (e.g. 0.95).
func WilsonScoreInterval(k, n int, confidence float64) ConfidenceInterval {
	if n <= 0 {
		return ConfidenceInterval{Confidence: confidence}
	}
	p := float64(k) / float64(n)

	// Approximate z-score for standard confidence levels
	z := 1.95996 // 95%
	if confidence >= 0.99 {
		z = 2.57583
	} else if confidence >= 0.90 && confidence < 0.95 {
		z = 1.64485
	}

	denom := 1.0 + (z*z)/float64(n)
	center := (p + (z*z)/(2.0*float64(n))) / denom
	margin := (z * math.Sqrt((p*(1.0-p))/float64(n)+(z*z)/(4.0*float64(n)*float64(n)))) / denom

	lower := math.Max(0.0, center-margin)
	upper := math.Min(1.0, center+margin)

	return ConfidenceInterval{
		Lower:      lower,
		Upper:      upper,
		Confidence: confidence,
	}
}

// PowerAnalysis computes the minimum sample size n required to detect a difference
// deltaMin between two groups with significance alpha and statistical power (1 - beta).
func PowerAnalysis(deltaMin, sigma float64, alpha, power float64) int {
	if deltaMin <= 0 || sigma <= 0 {
		return 1
	}
	zAlpha := 1.96 // alpha = 0.05 two-tailed
	if alpha <= 0.01 {
		zAlpha = 2.576
	} else if alpha >= 0.10 {
		zAlpha = 1.645
	}

	zBeta := 0.84 // power = 0.80
	if power >= 0.90 {
		zBeta = 1.282
	} else if power >= 0.95 {
		zBeta = 1.645
	}

	// n = 2 * ((z_alpha/2 + z_beta) * sigma / delta)^2
	factor := (zAlpha + zBeta) * sigma / deltaMin
	n := 2.0 * factor * factor
	return int(math.Ceil(n))
}

// ParetoPoint encapsulates the 4-dimensional outcome vector for quality-cost frontier analysis:
// (Success, Cost, Tokens, Latency). Higher success is better; lower cost, tokens, and latency are better.
type ParetoPoint struct {
	Name      string  `json:"name"`
	Budget    int     `json:"budget"`
	Success   float64 `json:"success"`    // maximize
	Cost      float64 `json:"cost"`       // minimize
	Tokens    float64 `json:"tokens"`     // minimize
	LatencyMs float64 `json:"latency_ms"` // minimize
	Dominated bool    `json:"dominated"`
}

// Dominates returns true if a dominates b:
// a is no worse than b in all 4 dimensions, and strictly better in at least one.
func (a ParetoPoint) Dominates(b ParetoPoint) bool {
	noWorse := a.Success >= b.Success &&
		a.Cost <= b.Cost &&
		a.Tokens <= b.Tokens &&
		a.LatencyMs <= b.LatencyMs

	strictlyBetter := a.Success > b.Success ||
		a.Cost < b.Cost ||
		a.Tokens < b.Tokens ||
		a.LatencyMs < b.LatencyMs

	return noWorse && strictlyBetter
}

// ComputeParetoFrontier identifies non-dominated configurations from a set of evaluated points.
func ComputeParetoFrontier(points []ParetoPoint) []ParetoPoint {
	m := len(points)
	result := make([]ParetoPoint, m)
	copy(result, points)

	for i := 0; i < m; i++ {
		result[i].Dominated = false
		for j := 0; j < m; j++ {
			if i == j {
				continue
			}
			if result[j].Dominates(result[i]) {
				result[i].Dominated = true
				break
			}
		}
	}
	return result
}
