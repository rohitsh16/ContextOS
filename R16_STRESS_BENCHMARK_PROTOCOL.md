# R16 Stress & Benchmark Protocol
## Proving Capability-Preserving Adaptive Inference Actually Works

This protocol is designed to falsify the R16 thesis, not merely demonstrate that the controller produces lower token counts.

The controller passes only if it can do both of these:

1. Spend less on tasks where extra compute is unnecessary.
2. Preserve or increase compute/model capability when the task genuinely requires it.

The central measured quantity is:

\[
\text{avoidable\_cost}
=
C_{\text{baseline}}
-
C_{\text{minimum sufficient}}
\]

subject to:

\[
Q_{\text{ContextOS}} \ge Q_{\min}
\]

and, for paired evaluation:

\[
P_{\text{ContextOS}} - P_{\text{baseline}}
\ge -\delta.
\]

---

# 1. Four Benchmark Rings

## Ring 0 — Controller Safety / Unit Stress

Purpose: prove the controller cannot systematically choose a cheaper but insufficient configuration.

No provider calls are needed.

Build hidden task states with a ground-truth capability surface:

```text
configuration -> true success probability -> true cost
```

The optimizer only sees noisy estimates.

Required scenarios:

```text
cheap + insufficient
cheap + sufficient
strong + sufficient
strongest + sufficient
retrieve + cheap + sufficient
think + cheap + insufficient
verify + medium + sufficient
escalate + strong + sufficient
```

### Mandatory adversarial tests

**Under-compute trap**

The cheapest configuration must be rejected because:

\[
Q_{LCB}<Q_{\min}.
\]

**Over-compute trap**

A high-cost configuration is unnecessary because a cheaper configuration satisfies the floor.

**Estimator-noise trap**

Perturb estimated quality by ±5%, ±10%, ±20%.

The controller should become more conservative as uncertainty increases.

**Cost-shock trap**

Multiply one provider's cost by:

```text
1.5x
2x
5x
10x
```

The controller may switch configurations, but must preserve the capability floor.

**Capability-shock trap**

Reduce the empirical success of the cheap model.

The controller must stop selecting it once its lower confidence bound falls below the floor.

### Ring 0 gates

```text
Capability floor violations: 0
Critical-task under-compute: 0
Negative-cost accounting events: 0
Budget overdrafts: 0
Invalid configurations selected: 0
```

---

# 2. Ring 1 — Large Synthetic Closed-Loop Stress

Purpose: test controller behavior over very large numbers of tasks before spending on real providers.

Recommended:

```text
10,000–100,000 synthetic tasks
20–100 concurrent workers
100+ task states
10+ cost regimes
10+ noise regimes
```

Every synthetic task has hidden truth that the optimizer never receives.

Generate independent variables:

```text
difficulty
information sufficiency
reasoning sensitivity
model capability
tool reliability
verification value
cache state
provider latency
cost
```

Use correlated task families so the controller cannot win by memorizing a single difficulty threshold.

### Stress distributions

Include:

```text
easy deterministic
easy but information-limited
easy but reasoning-sensitive
medium balanced
hard reasoning-sensitive
hard information-sensitive
critical verification-sensitive
critical escalation-sensitive
```

### Required perturbations

```text
confidence bias
difficulty misclassification
missing evidence
retrieval failure
tool timeout
provider timeout
cache miss
cost spike
latency spike
stale pricing
model quality drift
```

### Ring 1 metrics

For every task record:

```text
selected model
selected effort
retrieval actions
verification actions
escalations
actual synthetic quality
capability-floor violation
cost
oracle cost
oracle configuration
decision regret
```

Primary metrics:

\[
ViolationRate =
\frac{\#(Q<Q_{\min})}{N}
\]

\[
CostRegret =
\frac{C_{CTX}-C_{Oracle}}{C_{Oracle}}
\]

\[
ConfigurationRegret =
C_{CTX}-C_{Oracle}.
\]

---

# 3. Ring 2 — Replay Benchmark From Real Provider Traces

Purpose: remove network/provider noise while testing the controller against real usage distributions.

Run real tasks and record:

```text
request
model
effort
actual reasoning tokens
input tokens
cached tokens
cache writes
visible output
tool calls
latency
success
tests passed
verification result
actual billed cost when available
```

Then replay those observations through the optimizer.

Important:

The optimizer must not get the answer from the trace.

It may only get historical observations needed for estimation.

### Train / calibration split

```text
70% calibration
15% validation
15% holdout
```

Never tune the controller on the holdout.

### Ring 2 objective

Measure how closely the controller approaches the empirical oracle:

\[
ACR =
\frac{C_{CTX}}{C_{Oracle}}.
\]

Also report:

\[
\Delta Success =
Success_{CTX}-Success_{Baseline}.
\]

The controller should approach the cheapest configuration that satisfies the capability floor.

---

# 4. Ring 3 — Real Provider Execution

This is the first benchmark that can support a real-world cost claim.

Run the same task against:

```text
Baseline Default
Baseline Strong
Baseline Maximum/High
ContextOS
```

Whenever possible, use the same provider and model for within-model compute experiments.

Then separately evaluate model routing.

## Minimum canary

```text
30 tasks
5 T0
5 T1
7 T2
7 T3
6 T4
x 5 repeated runs where stochasticity is meaningful
```

= 150 task executions per policy.

## Full evaluation

```text
300+ unique tasks
5 repeats where needed
multiple repositories
multiple languages
held-out repository set
```

### Real-provider task success must be tied to objective evidence

For coding tasks:

```text
compiles
unit tests pass
integration tests pass
lint/static checks
patch correctness
no regression
```

For design/reasoning tasks:

```text
expert rubric
reference answer checks
constraint satisfaction
verification tests
```

Do not use model self-reported confidence as the ground-truth outcome.

---

# 5. Ring 4 — System Stress

Purpose: prove ContextOS remains correct under operational load.

Reuse the existing monorepo stress suite, but expand it.

The current monorepo test measures `Service.Plan()` latency under concurrency. That is useful, but it is not sufficient for R16 because it does not test capability preservation or real inference economics.

Add:

## A. Repository scale

```text
5k nodes / 25k edges
50k / 250k
100k / 500k
500k / 2.5M
1M / 5M
```

## B. Concurrency

```text
1
4
8
16
32
64
128 workers
```

## C. Query mix

```text
20% deterministic
20% easy
20% retrieval-heavy
20% reasoning-heavy
10% verification-heavy
10% adversarial
```

## D. Hot/cold patterns

```text
10% repeated hot queries
30% warm queries
60% cold queries
```

## E. Failure injection

Inject:

```text
1% provider timeout
5% provider timeout
10% provider timeout
1% retrieval failure
5% retrieval failure
cache misses
stale cache entries
cost metadata unavailable
```

---

# 6. Long-Horizon Stress

This is critical because a controller can appear cheap per turn while becoming expensive over many turns.

Run:

```text
10-turn
25-turn
50-turn
100-turn
```

tasks.

Measure cumulative:

\[
C_T = \sum_{t=1}^{T}
C_t
\]

and:

\[
Q_T
=
P(\text{task remains correct after }T\text{ turns}).
\]

Track:

```text
reasoning drift
context growth
cache degradation
retrieval repetition
verification frequency
escalation frequency
cost accumulation
failure accumulation
```

The controller must not save early and then lose the savings through repeated rediscovery or escalation.

---

# 7. Capability-Preservation Stress Matrix

This is the most important benchmark.

Construct task groups whose true sufficient-compute frontier is deliberately different.

| Task type | Cheap config | Strong config | Required behavior |
|---|---|---|---|
| Deterministic lookup | sufficient | sufficient | bypass |
| Easy edit | sufficient | sufficient | cheap |
| Ambiguous retrieval | insufficient | sufficient | retrieve |
| Logic-heavy bug | insufficient | sufficient | more reasoning |
| Deep concurrency | insufficient | sufficient | strong/high |
| Critical architecture | insufficient | sufficient | strongest + verification |
| Noisy evidence | uncertain | sufficient | conservative selection |
| Cost-shocked provider | sufficient | sufficient | cheaper model |
| Quality-degraded cheap model | insufficient | sufficient | escalate |

A controller that selects the same effort/model for all rows has failed the scientific objective even if its average cost is low.

---

# 8. Capability Floor Stress

For each task, establish:

\[
Q_{\min}(s)
=
Q_{\text{baseline}}(s)-\delta_s
\]

or a task-specific requirement.

Then record:

```text
Q_estimate
Q_LCB
Q_min
selected_cost
oracle_cost
```

The central audit table is:

| Task | Q_min | Q_LCB | Selected | Cost | Oracle | Floor Pass |
|---|---:|---:|---|---:|---:|---|
| easy-01 | 0.90 | 0.94 | cheap-low | ... | ... | PASS |
| hard-01 | 0.93 | 0.89 | cheap-low | ... | ... | FAIL |
| hard-01 | 0.93 | 0.95 | strong-high | ... | ... | PASS |

Any row like the second one is a controller safety failure.

---

# 9. Cost Stress

Do not only vary reasoning-token prices.

Run independent shocks to:

```text
input price
cached-input price
cache-write price
reasoning price
visible-output price
tool price
verification price
retry cost
escalation cost
```

Use multiplicative factors:

```text
0.5x
1x
2x
5x
10x
```

The optimizer should move toward the cheapest *sufficient* strategy after cost changes.

Capability must remain within the pre-registered floor.

---

# 10. Controller Overhead Stress

The controller itself must not become the expensive part.

Measure:

\[
\rho =
\frac{C_{controller}}
{C_{LLM}+C_{tools}+C_{controller}}.
\]

Also report CPU and wall time:

```text
controller p50
controller p95
controller p99
memory overhead
DB operations/query
graph expansions/query
```

Run with:

```text
1k
10k
100k
1M
```

repository graph sizes.

---

# 11. Oracle Comparison

For each benchmark task, enumerate the tested configuration set:

```text
model × effort × context × retrieval × verification
```

The oracle chooses:

\[
x^* =
\arg\min_x C(x)
\quad
\text{s.t.}
\quad
Q(x)\ge Q_{\min}.
\]

ContextOS does not know the future grid.

Then compute:

\[
ACR =
\frac{C_{CTX}}{C_{Oracle}}
\]

and:

\[
SuccessGap =
Q_{CTX}-Q_{Oracle}.
\]

The oracle is allowed to be unrealistic; its purpose is to establish how much economic regret remains.

---

# 12. Required Baselines

At minimum:

```text
B0 Fixed strong/default
B1 Fixed high reasoning
B2 Fixed maximum
B3 Context-only optimization
B4 Compute-only optimization
B5 Model routing only
B6 Context + compute
B7 Context + routing
B8 Full ContextOS capability-floor controller
B9 Offline oracle
```

Most important comparison:

```text
B8 with capability floor
vs
same optimizer with floor disabled
```

The floor-disabled arm tells us whether the claimed savings are actually coming from capability-preserving optimization.

---

# 13. Failure Taxonomy

Every failure must be classified.

```text
UNDER_COMPUTE
OVER_COMPUTE
UNDER_RETRIEVAL
OVER_RETRIEVAL
BAD_MODEL_ROUTE
BAD_EFFORT
BAD_STOPPING
BAD_VERIFICATION
BAD_ESCALATION
BAD_CALIBRATION
COST_ACCOUNTING
CACHE_SIDE_EFFECT
TOOL_FAILURE
PROVIDER_FAILURE
CONTROLLER_OVERHEAD
ORACLE_MISMATCH
```

The benchmark should produce a Pareto-style failure report, not just one pass/fail number.

---

# 14. Statistical Gates

For the final evaluation:

### Success

Use paired task outcomes.

Require the lower confidence bound of:

\[
Success_{CTX}-Success_{Baseline}
\]

to be above the pre-registered non-inferiority margin.

### Cost

Use paired bootstrap CIs on per-task cost.

Report:

```text
mean
median
P90
P95
bootstrap 95% CI
```

### Critical task safety

For T4:

```text
zero catastrophic under-compute events
```

or the project must explicitly document the observed failure and remain YELLOW/RED.

---

# 15. Green/Yellow/Red Gates

## GREEN

All of the following:

```text
capability non-inferiority passes
cost reduction is positive
critical under-compute = 0
holdout capability floor passes
real provider accounting reconciles
controller overhead is small
stress tests remain within SLA
oracle gap is bounded
```

## YELLOW

Examples:

```text
cost reduction exists but only on some providers
real-provider sample is too small
holdout is underpowered
capability floor is mostly met but uncertainty is too large
oracle gap remains large
```

## RED

Any of:

```text
cost savings mostly caused by capability degradation
critical task under-compute occurs
real billing contradicts estimated accounting
failure rate increases beyond non-inferiority margin
controller systematically selects insufficient cheap configurations
stress load causes correctness failures
```

---

# 16. Benchmark Output Format

Every execution should write JSONL.

One line per task:

```json
{
  "run_id": "uuid",
  "task_id": "T3_017",
  "policy": "contextos_capability_floor",
  "provider": "openai",
  "model": "model-version",
  "requested_effort": "high",
  "actual_reasoning_tokens": 8124,
  "input_tokens": 20143,
  "cached_input_tokens": 17120,
  "cache_write_tokens": 0,
  "visible_output_tokens": 432,
  "tool_calls": 4,
  "verification_calls": 1,
  "escalations": 0,
  "total_cost_usd": 0.1842,
  "quality": 0.97,
  "quality_lcb": 0.94,
  "capability_floor": 0.93,
  "capability_floor_pass": true,
  "success": true,
  "tests_passed": true,
  "latency_ms": 4210,
  "oracle_cost_usd": 0.1710
}
```

---

# 17. Minimum Report

Every run must output:

```text
Tasks
Success rate
Non-inferiority delta
Capability-floor violation rate
Critical under-compute count
Total cost
Cost/task
CPS
Oracle ACR
Reasoning tokens/task
Input tokens/task
Tool cost
Verification cost
Escalation cost
P50/P95/P99 latency
Controller overhead
```

And by class:

```text
T0
T1
T2
T3
T4
```

---

# 18. Recommended Execution Sequence

Do not jump directly to a 1,000-call real-provider run.

Run in this order:

```text
R16-S0  unit safety tests
R16-S1  10k synthetic tasks
R16-S2  100k synthetic stress
R16-S3  real trace replay
R16-S4  30-task real canary
R16-S5  150-task real benchmark
R16-S6  300+ task holdout benchmark
R16-S7  monorepo scale stress
R16-S8  long-horizon stress
R16-S9  cost/provider shock stress
R16-S10 final audit
```

Only proceed when the preceding gate is clean.

---

# 19. One Benchmark We Absolutely Need

The strongest falsification test is:

### Same model, same task, different reasoning

For each non-deterministic task run:

```text
minimal
low
medium
high
maximum
```

Then compare actual:

```text
quality
reasoning tokens
cost
latency
```

The empirical curve determines whether more reasoning was actually necessary.

Then let ContextOS select one point.

The controller gets credit only if it chooses a point that is:

1. above the capability floor, and
2. close to the minimum-cost sufficient point.

This directly tests the thesis without relying on hand-written difficulty thresholds.

---

# 20. Definition of "Actual Works"

We should not say R16 "works" merely because:

```text
tokens ↓
cost ↓
```

R16 works only when we observe:

```text
quality ≥ floor
AND
cost < baseline
AND
critical tasks remain safe
AND
holdout generalizes
AND
accounting reconciles
AND
stress does not break the controller.
```

That is the benchmark standard we should use for the research verdict.
