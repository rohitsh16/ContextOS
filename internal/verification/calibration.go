package verification

import (
	"math"
)

// PredictionObservation records a probabilistic confidence score alongside ground truth outcome.
type PredictionObservation struct {
	Confidence float64 `json:"confidence"` // p in [0, 1]
	Correct    bool    `json:"correct"`    // y in {0, 1}
}

// CalibrationReport contains statistical calibration metrics (R18.4 §50).
type CalibrationReport struct {
	Observations int     `json:"observations"`
	BrierScore   float64 `json:"brier_score"` // Mean squared error: sum((p - y)^2) / N
	ECE          float64 `json:"ece"`         // Expected Calibration Error across M bins
	MeanAccuracy float64 `json:"mean_accuracy"`
	MeanConfidence float64 `json:"mean_confidence"`
}

// ComputeCalibration calculates the empirical Brier score and ECE across 10 bins.
func ComputeCalibration(obs []PredictionObservation) CalibrationReport {
	n := len(obs)
	if n == 0 {
		return CalibrationReport{}
	}

	brierSum := 0.0
	correctCount := 0
	confSum := 0.0

	// 10 Bins: [0, 0.1), [0.1, 0.2), ..., [0.9, 1.0]
	const numBins = 10
	binCounts := make([]int, numBins)
	binCorrect := make([]int, numBins)
	binConfSum := make([]float64, numBins)

	for _, o := range obs {
		y := 0.0
		if o.Correct {
			y = 1.0
			correctCount++
		}
		conf := math.Max(0.0, math.Min(1.0, o.Confidence))
		confSum += conf

		diff := conf - y
		brierSum += diff * diff

		binIdx := int(conf * float64(numBins))
		if binIdx >= numBins {
			binIdx = numBins - 1
		}
		binCounts[binIdx]++
		if o.Correct {
			binCorrect[binIdx]++
		}
		binConfSum[binIdx] += conf
	}

	ece := 0.0
	for m := 0; m < numBins; m++ {
		if binCounts[m] == 0 {
			continue
		}
		acc := float64(binCorrect[m]) / float64(binCounts[m])
		avgConf := binConfSum[m] / float64(binCounts[m])
		weight := float64(binCounts[m]) / float64(n)
		ece += weight * math.Abs(acc-avgConf)
	}

	return CalibrationReport{
		Observations:   n,
		BrierScore:     brierSum / float64(n),
		ECE:            ece,
		MeanAccuracy:   float64(correctCount) / float64(n),
		MeanConfidence: confSum / float64(n),
	}
}

// SelectiveResult evaluates coverage vs risk at a confidence threshold (R18.3 §60).
type SelectiveResult struct {
	Threshold         float64 `json:"threshold"`
	Coverage          float64 `json:"coverage"`           // answered / total
	SelectiveAccuracy float64 `json:"selective_accuracy"` // correct / answered
	SelectiveRisk     float64 `json:"selective_risk"`     // 1 - selective_accuracy
	AnsweredCount     int     `json:"answered_count"`
	AbstainedCount    int     `json:"abstained_count"`
}

// EvaluateSelectiveAnswering evaluates the risk-coverage tradeoff for a given confidence threshold.
func EvaluateSelectiveAnswering(obs []PredictionObservation, threshold float64) SelectiveResult {
	if len(obs) == 0 {
		return SelectiveResult{}
	}

	answered := 0
	correctAnswered := 0

	for _, o := range obs {
		if o.Confidence >= threshold {
			answered++
			if o.Correct {
				correctAnswered++
			}
		}
	}

	total := len(obs)
	coverage := float64(answered) / float64(total)
	selAcc := 1.0
	if answered > 0 {
		selAcc = float64(correctAnswered) / float64(answered)
	}

	return SelectiveResult{
		Threshold:         threshold,
		Coverage:          coverage,
		SelectiveAccuracy: selAcc,
		SelectiveRisk:     1.0 - selAcc,
		AnsweredCount:     answered,
		AbstainedCount:    total - answered,
	}
}
