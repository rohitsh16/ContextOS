package compute

// PolicyTransition represents an effort level transition and its economic justification.
type PolicyTransition struct {
	Current        EffortLevel `json:"current"`
	Proposed       EffortLevel `json:"proposed"`
	ExpectedGain   float64     `json:"expected_gain"`
	TransitionCost float64     `json:"transition_cost"`
}

// AdaptiveController orchestrates the closed-loop decision cycle.
type AdaptiveController struct {
	profiler   *TaskProfiler
	stopping   *AdaptiveStopping
	calibrator *Calibrator
	voi        *VOIEngine
	router     *ModelRouter
	estimator  *ComputeEstimator
	optimizer  *CapabilityPreservingOptimizer

	hysteresisDelta float64
	turnThreshold   float64
}

// NewAdaptiveController initializes the controller with coordinated reasoning subsystems.
func NewAdaptiveController() *AdaptiveController {
	return &AdaptiveController{
		profiler:        NewTaskProfiler(),
		stopping:        NewAdaptiveStopping(0.05, 15),
		calibrator:      NewCalibrator(),
		voi:             NewVOIEngine(DefaultUtilityWeights()),
		router:          NewModelRouter(),
		estimator:       NewComputeEstimator(0.50),
		optimizer:       NewCapabilityPreservingOptimizer(NewSyntheticCapabilityEstimator(), DefaultModels()),
		hysteresisDelta: 0.05,
		turnThreshold:   0.10,
	}
}

// SetOptimizer configures the capability-preserving optimizer (e.g. empirical or custom).
func (ac *AdaptiveController) SetOptimizer(opt *CapabilityPreservingOptimizer) {
	if opt != nil {
		ac.optimizer = opt
	}
}

// DecisionStepResult contains the chosen action, updated effort, and explanation.
type DecisionStepResult struct {
	Action        Action                 `json:"action"`
	StopReason    string                 `json:"stop_reason,omitempty"`
	Effort        EffortLevel            `json:"effort"`
	CandidateVOI  []CandidateActionScore `json:"candidate_voi"`
	Model         ModelCandidate         `json:"model"`
	Configuration *Configuration         `json:"configuration,omitempty"`
}

// StepPreserving evaluates state with R16 capability floor and selects the minimum-sufficient configuration.
func (ac *AdaptiveController) StepPreserving(
	state ControllerState,
	task TaskProfile,
	floor CapabilityFloor,
	currentEffort EffortLevel,
) DecisionStepResult {
	// 1. Calibrate confidence and risk
	calibratedConf := ac.calibrator.CalibrateConfidence(
		state.EstimatedConfidence,
		state.Difficulty,
		state.EvidenceCoverage,
		state.EvidenceConflict,
	)
	state.EstimatedConfidence = calibratedConf
	state.CalibratedRisk = ac.calibrator.CalibrateRisk(calibratedConf, state.Risk)

	// 2. Select minimum-sufficient configuration satisfying capability floor (Invariant A, B, C, D)
	cfg, _ := ac.optimizer.SelectMinimumSufficient(state, task, floor)

	// 3. Evaluate candidate action VOIs
	actionScores := ac.voi.EvaluateActions(
		state.EstimatedConfidence,
		state.CalibratedRisk,
		state.EvidenceCoverage,
		state.Difficulty,
		state.CacheState,
		state.RemainingBudget,
	)

	best := ac.voi.BestAction(actionScores)

	// Invariant C: if floor mandates verification and we haven't verified, prioritize verification before stop
	if floor.RequireVerification && best.Action == ActionStop && !state.Verified {
		best.Action = ActionVerify
	}

	// 4. Check adaptive stopping conditions
	if best.Action == ActionStop {
		stop, reason := ac.stopping.ShouldStop(state, best.VOI)
		if stop {
			return DecisionStepResult{
				Action:        ActionStop,
				StopReason:    reason,
				Effort:        cfg.Effort,
				CandidateVOI:  actionScores,
				Model:         cfg.Model,
				Configuration: &cfg,
			}
		}
	}

	return DecisionStepResult{
		Action:        best.Action,
		Effort:        cfg.Effort,
		CandidateVOI:  actionScores,
		Model:         cfg.Model,
		Configuration: &cfg,
	}
}

// Step evaluates current state and selects the next optimal action.
func (ac *AdaptiveController) Step(
	state ControllerState,
	policy ComputePolicy,
	currentEffort EffortLevel,
) DecisionStepResult {
	// 1. Calibrate confidence and risk
	calibratedConf := ac.calibrator.CalibrateConfidence(
		state.EstimatedConfidence,
		state.Difficulty,
		state.EvidenceCoverage,
		state.EvidenceConflict,
	)
	state.EstimatedConfidence = calibratedConf
	state.CalibratedRisk = ac.calibrator.CalibrateRisk(calibratedConf, state.Risk)

	// 2. Select model based on difficulty
	model := ac.router.SelectOptimalModel(state.Difficulty, "")

	// 3. Evaluate candidate action VOIs
	actionScores := ac.voi.EvaluateActions(
		state.EstimatedConfidence,
		state.CalibratedRisk,
		state.EvidenceCoverage,
		state.Difficulty,
		state.CacheState,
		state.RemainingBudget,
	)

	best := ac.voi.BestAction(actionScores)

	// 4. Check adaptive stopping conditions
	stop, reason := ac.stopping.ShouldStop(state, best.VOI)
	if stop {
		return DecisionStepResult{
			Action:       ActionStop,
			StopReason:   reason,
			Effort:       currentEffort,
			CandidateVOI: actionScores,
			Model:        model,
		}
	}

	// 5. Check Turn Minimization: if TurnValue <= turnThreshold, terminate
	turnCost := 0.05 // Baseline amortized turn cost
	turnValue := best.ExpectedGain / (turnCost + best.ExpectedCost)
	if state.TurnCount > 3 && turnValue < ac.turnThreshold {
		return DecisionStepResult{
			Action:       ActionStop,
			StopReason:   "turn_minimization: marginal turn value below threshold",
			Effort:       currentEffort,
			CandidateVOI: actionScores,
			Model:        model,
		}
	}

	// 6. Check effort transition and policy hysteresis
	recommendedEffort := currentEffort
	if best.Action == ActionThink {
		curve := ac.estimator.EstimateCurve(model.Provider, model.Model, state.Difficulty)
		optEffort := ac.estimator.RecommendOptimalEffort(curve)

		// Hysteresis rule: Only shift effort if expected gain exceeds transition cost + delta
		transitionCost := 0.02
		expectedGain := float64(optEffort-currentEffort) * 0.15
		if expectedGain > (transitionCost + ac.hysteresisDelta) {
			recommendedEffort = optEffort
		}
	}

	return DecisionStepResult{
		Action:       best.Action,
		Effort:       recommendedEffort,
		CandidateVOI: actionScores,
		Model:        model,
	}
}
