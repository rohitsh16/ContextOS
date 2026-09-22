# R16 — Capability-Preserving Adaptive Inference
## Implementation and Validation Plan for ContextOS

## 0. Objective

ContextOS must optimize **avoidable end-to-end inference cost** without reducing the capability available to the model when the task requires it.

The target is not:

> minimize reasoning tokens.

The target is:

> minimize expected end-to-end cost subject to a measured capability/quality floor.

For task state `s`, model `m`, inference configuration `e` and policy `π`:

\[
\pi^* =
\arg\min_\pi \mathbb{E}[C_{E2E}(\pi,s)]
\]

subject to

\[
Q(\pi,s) \ge Q_{\min}(s)
\]

and, for probabilistic task success,

\[
P(\text{correct}\mid \pi,s)
\ge
P_{\text{baseline}}(\text{correct}\mid s)-\delta_s.
\]

`δ_s` is a pre-registered non-inferiority margin. For critical tasks, use a stricter margin.

This turns ContextOS into a **capability-preserving inference controller** rather than a token minimizer.

---

## 1. Hard Invariants

These are non-negotiable acceptance rules.

### Invariant A — No capability-for-cost trade below the floor

The controller may reduce compute only when the measured/predicted quality remains above the task's required floor.

It must never choose:

`cheaper → lower quality`

when the cheaper configuration falls below the floor.

### Invariant B — Under uncertainty, preserve compute

When uncertainty in the quality estimate is large, ContextOS must err toward the higher-capability configuration.

Operational rule:

\[
\hat Q_{LCB}(m,e,s) < Q_{\min}(s)
\Rightarrow
\text{configuration is ineligible}
\]

where `LCB` is a lower confidence bound.

### Invariant C — Hard tasks are allowed to consume more compute

A difficult or high-risk task may receive:

- more reasoning,
- more retrieval,
- a stronger model,
- verification,
- escalation,
- additional turns,

when those actions are needed to stay above the capability floor.

There is no global budget rule that forces a difficult task into a cheap regime.

### Invariant D — Savings should come from avoidable over-allocation

Define:

\[
C_{\text{avoidable}} =
C_{\text{baseline}} - C_{\text{minimum-sufficient}}.
\]

The project succeeds when it removes avoidable spend, not when it merely makes the model weaker.

---

# 2. What the Current Repo Tells Us

The current implementation has several mechanisms that must be replaced or isolated before claiming real cost optimization.

## 2.1 Synthetic reasoning curves

`internal/compute/estimator.go` currently creates a fixed effort ladder:

- minimal = 0 reasoning tokens
- low = 2,048
- medium = 8,192
- high = 16,384
- maximum = 32,768

It then synthesizes success probability from task difficulty.

This is useful as a controller prototype, but it is not evidence about a real model's reasoning frontier.

### Required change

Keep the estimator interface, but introduce two implementations:

- `SyntheticCapabilityEstimator` — retained only for unit tests/controller simulation.
- `EmpiricalCapabilityEstimator` — populated from observed benchmark runs.

The production research path must use empirical observations once real providers are connected.

---

## 2.2 Provider `Generate()` is mock/offline

The current OpenAI, Anthropic and Gemini `Generate()` functions synthesize token usage instead of invoking the real providers.

This means all existing cost results are mechanism/simulation results rather than verified provider-billed cost.

### Required change

Introduce:

```text
Provider
  ├── RealProvider
  │     ├── OpenAI
  │     ├── Anthropic
  │     └── Gemini
  └── MockProvider
```

The mock implementation remains useful for deterministic controller tests.

The research benchmark must explicitly label runs as:

- `synthetic`
- `mock_provider`
- `real_provider`

and never mix them in one aggregate.

---

# 3. New Core Abstraction: Capability Envelope

Add a normalized representation of the capability available from a configuration.

Suggested file:

`internal/compute/capability.go`

```go
type CapabilityEnvelope struct {
    Provider string
    Model    string

    Effort EffortLevel

    // Predicted / empirical quality.
    MeanQuality float64

    // Lower confidence bound used for safety.
    QualityLCB float64

    // Probability of meeting the task success criterion.
    SuccessProbability float64
    SuccessLCB         float64

    // Cost expected for one execution.
    ExpectedCostUSD float64

    // Expected total latency.
    ExpectedLatencyMS float64

    // Uncertainty of the estimator.
    QualityStdErr float64

    // Number of observations supporting this estimate.
    Samples int
}
```

The key rule is that the controller chooses from envelopes, not from raw token counts.

---

# 4. Capability Floor

Add:

`internal/compute/capability_floor.go`

```go
type CapabilityFloor struct {
    RequiredSuccessProbability float64

    // Optional task-specific quality score.
    RequiredQuality float64

    // Maximum tolerated regression vs baseline.
    NonInferiorityMargin float64

    // Extra conservatism for high-risk tasks.
    RiskMultiplier float64
}
```

A resolver maps task state to the floor:

```go
func ResolveCapabilityFloor(
    task TaskProfile,
    riskTarget float64,
) CapabilityFloor
```

Suggested policy:

| Task class | Initial floor strategy |
|---|---|
| T0 deterministic | exact-result invariant rather than model quality |
| T1 trivial | baseline non-inferiority |
| T2 moderate | baseline non-inferiority |
| T3 difficult | stricter non-inferiority |
| T4 critical | strictest observed baseline + verification requirement |

Do not hard-code a universal success number until the empirical benchmark is built.

---

# 5. Baseline Definition

Every ContextOS configuration must be compared against a capability baseline.

Baseline candidates:

1. same model, standard/default effort
2. same model, maximum available reasoning
3. current agent's existing policy
4. strong-model fixed-compute baseline

The primary comparison should use the **same underlying task, context, model family and tool access** whenever possible.

For each task, preserve the baseline response/result so capability can be compared paired rather than only by aggregate averages.

---

# 6. Empirical Capability Surface

Replace the synthetic curve

\[
A_m(e)
\]

with an observed surface:

\[
Q(s,m,e,c,t)
\]

where:

- `s` = task state
- `m` = model
- `e` = effort / reasoning configuration
- `c` = context/retrieval configuration
- `t` = tool/verification configuration

The important observation is that reasoning is not independent of context.

The controller is actually optimizing over a joint surface:

\[
\mathcal{F}(s)=
\{(m,e,c,t,Q,C,L)\}.
\]

The admissible set is:

\[
\mathcal{A}(s)=
\{x: Q_{LCB}(x,s)\ge Q_{\min}(s)\}.
\]

Then:

\[
x^* =
\arg\min_{x\in\mathcal{A}(s)}
\mathbb{E}[C_{E2E}(x,s)].
\]

This is the central optimization problem for R16.

---

# 7. Minimum-Sufficient Reasoning

Define the minimum sufficient reasoning level empirically:

\[
R^*(s,m)=
\min_R
\left\{
R:
Q_{LCB}(s,m,R)\ge Q_{\min}(s)
\right\}.
\]

Important:

`R*` is not assumed to be the smallest token count.

It may be:

- low for simple tasks,
- medium for some tasks,
- high or maximum for difficult tasks,
- adaptive/unbounded for models that internally decide how much reasoning to use.

For adaptive-thinking models, record **actual reasoning tokens** rather than treating the requested budget as the realized compute.

---

# 8. Replace the Current Effort Transition Logic

The current controller computes a synthetic transition gain based on the integer difference between effort levels.

That is not sufficient.

New controller decision:

```go
type Configuration struct {
    Model  ModelCandidate
    Effort EffortLevel

    MaxReasoningTokens *int64
    MaxOutputTokens    *int64

    RetrieveBudget int
    VerifyEnabled  bool
}
```

Generate feasible configurations, then filter by capability floor:

```go
func (o *Optimizer) FeasibleConfigurations(
    state ControllerState,
) []Configuration
```

Then:

```go
func (o *Optimizer) SelectMinimumSufficient(
    state ControllerState,
) Configuration
```

Selection rule:

```text
1. Estimate quality LCB for every feasible configuration.
2. Remove configurations below the capability floor.
3. Estimate complete E2E cost.
4. Select the lowest-cost eligible configuration.
5. If uncertainty is too high, choose the safer higher-capability configuration.
```

This is fundamentally different from "pick the cheapest model."

---

# 9. Correct VOI Accounting

The current `internal/compute/voi.go` subtracts action cost inside `ComputeUtility` and then subtracts it again when computing VOI for several actions.

That can double-charge action cost.

Current pattern:

```go
uAfter := ComputeUtility(... budget.SpentCostUSD+cost ...)
voi := uAfter - currentUtility - cost
```

Fix to:

```go
voi := uAfter - currentUtility
```

or, preferably, refactor utility so action deltas are represented exactly once.

Use one canonical function:

```go
func ActionDelta(
    current StateValue,
    projected StateValue,
) float64
```

and make accounting tests assert:

```text
VOI = projected utility - current utility
```

with no duplicated monetary penalty.

---

# 10. Full End-to-End Cost Model

Cost must be decomposed into:

\[
C_{E2E}
=
C_{input}
+
C_{cache}
+
C_{cachewrite}
+
C_{reasoning}
+
C_{visible}
+
C_{tools}
+
C_{verification}
+
C_{retries}
+
C_{escalation}
+
C_{controller}.
\]

Store the decomposition per task.

Suggested telemetry:

```go
type CostBreakdown struct {
    InputUSD          float64
    CachedInputUSD    float64
    CacheWriteUSD     float64
    ReasoningUSD      float64
    VisibleOutputUSD  float64
    ToolUSD           float64
    VerificationUSD   float64
    RetryUSD          float64
    EscalationUSD     float64
    ControllerUSD     float64

    TotalUSD          float64
}
```

Do not report only aggregate estimated cost.

---

# 11. Separate Requested Compute from Realized Compute

This distinction is essential.

Store both:

```text
requested_effort
requested_reasoning_budget
actual_reasoning_tokens
actual_output_tokens
actual_total_tokens
```

For adaptive providers, the requested setting is a policy constraint, not proof of consumed compute.

The benchmark should calculate:

\[
\text{reasoning utilization}
=
\frac{R_{\text{actual}}}{R_{\text{budget}}}
\]

when a finite budget exists.

---

# 12. Model Routing With Capability Constraints

The current router chooses using hand-written expected costs and historical success rates.

Replace:

\[
m^* = \arg\min_m \frac{E[C_m]}{P_m}
\]

with constrained configuration selection:

\[
(m^*,e^*) =
\arg\min_{m,e}
E[C_{E2E}(m,e)]
\]

subject to

\[
P_{\text{LCB}}(\text{success}\mid m,e,s)
\ge
Q_{\min}(s).
\]

This prevents a cheaper model from being selected merely because its nominal cost is low.

Routing should also support:

```text
cheap-but-sufficient
strong-but-sufficient
strongest-required
```

but these labels are only policy shortcuts. The actual decision must come from measured envelopes.

---

# 13. Escalation and Capability Insurance

Do not spend the entire budget at the beginning.

Maintain reserved capacity for uncertainty.

The existing budget manager already has:

- verification reserve
- escalation reserve

Keep that structure but make reserves capability-aware.

Example:

```text
initial attempt
    ↓
observe evidence + actual quality signals
    ↓
still above floor?
    ├── yes → stop
    └── no / uncertain
            ↓
       retrieve / verify / think / escalate
```

For a hard task, the optimizer may deliberately choose an expensive initial configuration to reduce the probability of needing an even more expensive recovery path.

This is expected-cost optimization, not per-request token minimization.

---

# 14. Deterministic Bypass Must Be Capability-Proven

`internal/verification/risk.go` currently allows deterministic bypass for classes such as:

- symbol definition lookup
- caller graph lookup
- import lookup
- interface implementation lookup
- git diff

The bypass is valid only when the underlying operation has an exact oracle.

Do not treat "this looks deterministic" as proof.

Required invariant:

```text
bypass result == oracle result
```

for every bypassed task in the evaluation suite.

For T0 tasks, model reasoning is irrelevant because the task is solved by a deterministic computation.

This is not "reducing model capability"; it is avoiding unnecessary model invocation.

---

# 15. Provider Adapter Architecture

Add explicit provider modes:

```go
type ProviderMode string

const (
    ProviderModeMock ProviderMode = "mock"
    ProviderModeReal ProviderMode = "real"
)
```

The real adapter must:

1. send the actual provider request
2. collect raw provider usage
3. normalize it
4. preserve raw usage for audit
5. calculate an estimated cost
6. record provider-reported billing/cost when available
7. record any fields ContextOS cannot normalize

Never overwrite provider-reported usage with synthetic values.

---

# 16. Pricing Registry

Move pricing data out of hard-coded controller assumptions.

Suggested:

`internal/telemetry/pricing_registry.go`

```go
type PricingEntry struct {
    Provider string
    Model    string

    EffectiveAt time.Time
    Source      string
    Version     string

    InputPerMillion       float64
    CachedInputPerMillion float64
    CacheWritePerMillion  float64
    OutputPerMillion      float64
    ReasoningPerMillion   float64
}
```

The benchmark manifest must pin:

```text
provider
model
pricing version
pricing effective date
currency
billing unit
source
```

Pricing updates must not silently rewrite historical benchmark results.

---

# 17. Experimental Dataset

Create a new R16 matrix.

Target:

- 150–300 tasks initially
- balanced across T0–T4
- multiple repositories
- multiple languages where practical
- easy/medium/hard slices
- adversarial/OOD holdout kept private until evaluation

Each task should run paired:

```text
Baseline
ContextOS
```

and, where feasible:

```text
Baseline-Max
Baseline-Default
ContextOS-Minimum-Sufficient
ContextOS-Oracle
```

Do not tune the controller on the holdout.

---

# 18. Core R16 Benchmark Grid

For each task:

### Fixed model / varying compute

```text
minimal
low
medium
high
maximum
adaptive
```

Measure:

- success
- quality
- reasoning tokens
- input tokens
- output tokens
- cost
- latency

### Fixed compute / varying model

Measure the same metrics.

### Joint optimization

Allow ContextOS to select:

```text
model
effort
context
retrieval
verification
turn count
```

---

# 19. Primary Metrics

The primary metric is:

\[
CPS = \frac{\text{total cost}}{\text{successful tasks}}.
\]

Also report:

### Capability preservation

\[
\Delta Q = Q_{ContextOS}-Q_{baseline}.
\]

### Cost reduction

\[
\Delta C\% =
1-
\frac{C_{ContextOS}}{C_{baseline}}.
\]

### Failure rate

\[
F = 1-P(\text{success}).
\]

### Minimum sufficient compute

\[
R^*(s,m)
\]

for each task/model where identifiable.

### Tail cost

P50, P90, P95 task cost.

### Tail latency

P50, P90, P95.

---

# 20. Acceptance Criteria

Do not use a single cost number as the success criterion.

A run can advance only if:

### Capability condition

ContextOS is statistically non-inferior to the selected baseline:

\[
P_{CTX}-P_{BASE} \ge -\delta.
\]

For critical tasks:

\[
\delta_{critical} < \delta_{general}.
\]

### Cost condition

Total E2E cost decreases.

### Robustness condition

The same capability guarantee holds on the unseen holdout.

### No hidden subsidy

Controller compute, failed attempts, retries and verification are included in total cost.

### Measurement integrity

Real provider usage is available for any claim labeled as real-world cost.

---

# 21. Statistical Procedure

For paired task outcomes, use paired bootstrap confidence intervals.

For success-rate non-inferiority:

- paired task bootstrap
- Wilson / Clopper–Pearson intervals as appropriate for binomial slices
- pre-registered non-inferiority margin

For cost:

- paired bootstrap on per-task cost
- report median and mean
- report P90/P95 because a few escalations may dominate tail cost

For multiple task classes:

- report all-class aggregate
- report T0/T1/T2/T3/T4 separately

Do not hide capability regressions in aggregate averages.

---

# 22. Ablations

Required ablations:

1. context optimization only
2. compute optimization only
3. model routing only
4. context + compute
5. context + routing
6. compute + routing
7. full joint controller
8. full controller without capability floor
9. full controller without escalation reserve
10. full controller without verification reserve
11. full controller with synthetic estimator
12. full controller with empirical estimator

The most important comparison is:

```text
with capability floor
vs
without capability floor
```

This demonstrates whether cost gains come from true minimum-sufficient inference or simply from lowering reasoning.

---

# 23. Oracle Analysis

Construct an offline oracle from the measured task-level grid.

For each task, the oracle knows the observed result of every tested configuration.

The oracle selects the least-cost configuration that satisfies the capability floor.

Then compare:

\[
C_{CTX}
\]

against

\[
C_{Oracle}.
\]

Define:

\[
ACR =
\frac{C_{CTX}}{C_{Oracle}}.
\]

An ACR near 1 means the controller is approaching the empirical minimum-cost frontier.

Do not use an oracle built from synthetic curves to claim superiority.

---

# 24. Safety Test for the Controller

Construct counterexamples where the cheapest configuration is insufficient.

Examples:

```text
cheap model + low reasoning → failure
strong model + medium reasoning → success
strongest model + high reasoning → success
```

The controller must reject the cheap configuration when its lower confidence bound is below the capability floor.

Then construct the opposite case:

```text
cheap model + low reasoning → success
strong model + high reasoning → success
```

The controller should be able to select the cheaper sufficient configuration.

These paired tests validate that the mechanism is sensitive to **sufficiency**, not merely cost.

---

# 25. Realistic Cost Scenarios

At minimum include:

### Scenario A — Reasoning-dominant

Large reasoning token consumption and small context.

### Scenario B — Context-dominant

Large prompt/cached-context cost and modest reasoning.

### Scenario C — Tool-dominant

Many tool invocations and retries.

### Scenario D — Verification-dominant

Initial answer is cheap, but verification is expensive.

### Scenario E — Escalation-dominant

Cheap first attempt frequently escalates on difficult tasks.

### Scenario F — Cache-sensitive

Changing context or compute configuration affects reuse.

This prevents the controller from overfitting to one cost source.

---

# 26. Controller Objective

Replace a single heuristic utility with a constrained optimization objective.

Recommended formulation:

\[
J(\pi;s)
=
\mathbb{E}[C_{E2E}(\pi,s)]
+
\lambda_F
\mathbb{P}
\left(
Q(\pi,s)<Q_{\min}(s)
\right)
\]

with sufficiently large `λ_F`.

Equivalent constrained form:

\[
\min_\pi \mathbb{E}[C_{E2E}]
\quad
\text{s.t.}
\quad
Q_{LCB}\ge Q_{\min}.
\]

The constrained formulation is preferred for the primary implementation because it makes the capability requirement explicit.

---

# 27. Future Research Direction: Chance-Constrained Inference

Once empirical data exists, extend the controller to:

\[
\min_x E[C(x)]
\]

subject to

\[
P(Q(x)\ge Q_{\min})\ge 1-\alpha.
\]

This is stronger than deterministic thresholding because model performance and task difficulty are uncertain.

The practical implementation can use a conservative lower confidence bound first, then move toward calibrated chance constraints after enough data is collected.

---

# 28. Future Research Direction: Distributional Cost/Capability

Do not model cost or quality only by their mean.

Store:

\[
C(x)\sim \mathcal{D}_C
\]

and

\[
Q(x)\sim \mathcal{D}_Q.
\]

Then optimize a risk-aware objective such as:

\[
E[C]+\eta \operatorname{CVaR}_{\beta}(C)
\]

while enforcing the capability constraint.

This becomes important when escalation or verification creates heavy-tailed cost distributions.

---

# 29. Exact Implementation Order

## Phase R16.1 — Measurement Integrity

1. Add benchmark execution IDs.
2. Add provider mode (`mock`/`real`).
3. Persist raw provider usage.
4. Normalize actual reasoning tokens.
5. Add cost breakdown.
6. Add pricing version/source.
7. Add requested-vs-realized compute fields.

### Exit condition

No synthetic value can be mistaken for actual provider usage.

---

## Phase R16.2 — Accounting Correctness

1. Fix VOI double charging.
2. Add cost accounting unit tests.
3. Add budget reserve tests.
4. Verify effective input token arithmetic.
5. Verify reasoning/output accounting.
6. Verify retries and verification are included exactly once.

### Exit condition

Independent recomputation of every benchmark total matches reported totals.

---

## Phase R16.3 — Empirical Capability Curves

1. Run fixed-model effort sweeps.
2. Run fixed-effort model sweeps.
3. Estimate quality distributions.
4. Compute confidence intervals.
5. Compute `R*`.
6. Identify task classes with compute-sensitive behavior.

### Exit condition

The system has measured, not invented, capability curves.

---

## Phase R16.4 — Capability Floor

1. Implement `CapabilityFloor`.
2. Implement quality lower confidence bounds.
3. Implement configuration feasibility filtering.
4. Reject below-floor options.
5. Test cheap-but-insufficient counterexamples.
6. Test cheap-and-sufficient cases.

### Exit condition

No cost-saving decision can bypass the floor.

---

## Phase R16.5 — Empirical Joint Optimizer

Optimize jointly over:

```text
model
effort
context
retrieval
verification
turns
escalation
```

using measured task-conditioned data.

### Exit condition

Joint controller beats fixed policies on cost while remaining non-inferior on capability.

---

## Phase R16.6 — OOD / Robustness

Evaluate on held-out repositories and unseen task templates.

Stress:

- distribution shift
- misleading difficulty features
- noisy confidence estimates
- stale cost estimates
- changed provider latency
- changed pricing
- cache misses
- failed tools

### Exit condition

Capability guarantees remain stable under holdout shift.

---

## Phase R16.7 — Final Verdict

Possible outcomes:

### GREEN

Measured real-provider E2E cost decreases and capability is non-inferior with robust holdout results.

### YELLOW

Cost decreases but evidence is incomplete, synthetic, underpowered or provider-specific.

### RED

Cost reduction is primarily obtained by capability degradation, or the controller cannot reliably identify minimum-sufficient configurations.

---

# 30. Files to Add / Modify

### Add

```text
internal/compute/capability.go
internal/compute/capability_floor.go
internal/compute/optimizer.go
internal/telemetry/cost_breakdown.go
internal/telemetry/pricing_registry.go
internal/providers/mode.go
```

### Modify

```text
internal/compute/controller.go
internal/compute/estimator.go
internal/compute/voi.go
internal/compute/routing.go
internal/compute/budget.go
internal/planning/compute_plan.go
internal/providers/request.go
internal/providers/*/*.go
internal/telemetry/usage.go
```

### Add tests

```text
internal/compute/capability_floor_test.go
internal/compute/optimizer_test.go
internal/compute/voi_accounting_test.go
internal/telemetry/cost_breakdown_test.go
internal/providers/provider_mode_test.go
benchmarks/compute/r16_capability_test.go
benchmarks/compute/r16_cost_test.go
benchmarks/compute/r16_ood_test.go
```

---

# 31. What We Should Not Do

Do not:

- optimize only reasoning-token count
- compare only model API prices
- use hand-written success rates as empirical evidence
- treat requested thinking budget as actual thinking cost
- call a provider integration "real" while `Generate()` is still mock
- claim a capability-preserving result from average success rates alone
- tune against the holdout
- let a deterministic bypass claim "model reasoning savings" without exact oracle validation
- migrate the database or redesign unrelated subsystems before measurement shows the need

---

# 32. First Concrete Research Run

The first real R16 experiment should be intentionally small and auditable.

### Matrix

- 30 tasks
- 6 task slices across T0–T4
- 3 models
- 5 effort settings where supported
- 5 repeated trials per cell where stochasticity matters

### Compare

```text
Baseline fixed configuration
ContextOS heuristic
ContextOS capability-floor optimizer
Oracle
```

### Report

```text
success rate
non-inferiority CI
input cost
reasoning cost
visible output cost
tool cost
verification cost
retry cost
escalation cost
total E2E cost
CPS
P50/P95 latency
actual reasoning tokens
```

### Decision

Do not advance to larger runs until:

1. accounting reconciles exactly,
2. capability-floor tests pass,
3. the controller can choose both:
   - cheap-and-sufficient
   - expensive-but-required
4. no capability regression is hidden by aggregate averages.

---

# 33. Research Thesis After R16

The strongest version of the ContextOS research thesis is:

> ContextOS performs capability-preserving adaptive inference by jointly selecting context, retrieval, model, reasoning effort, verification and escalation so that the lowest expected end-to-end cost configuration satisfying a task-conditioned capability constraint is selected.

The technical novelty, if the experiments support it, is not "use less reasoning."

It is the **closed-loop identification and enforcement of a minimum-sufficient inference frontier across heterogeneous models, context and verification actions under uncertain cost and quality.**
