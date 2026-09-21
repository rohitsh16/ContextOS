package compute

// Calibrator adjusts subjective model confidence and risk metrics into empirically grounded probabilities.
type Calibrator struct {
	shrinkageFactor float64
}

// NewCalibrator creates a calibrator with default shrinkage factor.
func NewCalibrator() *Calibrator {
	return &Calibrator{shrinkageFactor: 0.25}
}

// CalibrateConfidence adjusts raw model confidence using task difficulty, evidence coverage, and conflict.
// LLMs tend to be overconfident; this performs empirical shrinkage towards reality.
func (c *Calibrator) CalibrateConfidence(rawConfidence, difficulty, evidenceCoverage, evidenceConflict float64) float64 {
	// Base discount from unobserved evidence
	coveragePenalty := (1.0 - evidenceCoverage) * 0.40

	// Penalty from conflicting evidence
	conflictPenalty := evidenceConflict * 0.35

	// High difficulty dampens subjective confidence
	difficultyPenalty := difficulty * 0.20

	calibrated := rawConfidence - coveragePenalty - conflictPenalty - difficultyPenalty

	// Empirical Bayesian shrinkage towards a conservative prior of 0.50
	calibrated = (1.0-c.shrinkageFactor)*calibrated + (c.shrinkageFactor * 0.50)

	if calibrated < 0.05 {
		calibrated = 0.05
	}
	if calibrated > 0.99 {
		calibrated = 0.99
	}
	return calibrated
}

// CalibrateRisk converts calibrated confidence, difficulty, and impact into residual error risk P(error) * Impact.
func (c *Calibrator) CalibrateRisk(calibratedConfidence, taskRisk float64) float64 {
	probError := 1.0 - calibratedConfidence
	if probError < 0 {
		probError = 0
	}
	// Risk = P(error) * Impact
	risk := probError * (0.5 + 0.5*taskRisk)
	if risk > 1.0 {
		risk = 1.0
	}
	return risk
}
