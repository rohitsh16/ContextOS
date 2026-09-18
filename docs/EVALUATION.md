# ContextOS evaluation plan

## Primary question

Can ContextOS reach a fixed software-engineering task-success target with less context and lower total model cost than strong baselines?

## Baselines

- fresh agent
- full historical transcript
- semantic top-k retrieval
- repository-aware retrieval
- persistent memory
- adaptive trajectory compression
- ContextOS deterministic allocator
- ContextOS learned allocator (future)

## Longitudinal task protocol

1. Agent A works on a real repository task.
2. Stop and persist state.
3. Modify repository state or branch where appropriate.
4. Start Agent B or another model.
5. Require continuation without manually copying the previous transcript.
6. Measure task success, context, tokens, latency and cost.

## Budget sweep

Evaluate budgets such as:

```text
512, 1024, 2048, 4096, 8192, 16384, 32768
```

Report the minimum budget that reaches target success.

## Metrics

### Quality

- Pass@1 / task success
- regression rate
- handoff success
- rediscovery rate

### Context efficiency

$$
CE = \frac{\text{Success}}{\text{InputTokens} / 1000}
$$

$$
B_\tau = \min \{ B : P(\text{success}) \ge \tau \}
$$

### Memory health

- stale-context rate
- contradictory-memory rate
- memory precision/recall

### Cache

- cache hit rate
- cached token ratio
- cache-adjusted cost

### Latency

- retrieval latency
- TTFT
- end-to-end task latency

## Counterfactual context utility

For a completed task with context set $C$, estimate the marginal value of an item $m$ using ablation:

$$
\Delta_m = U(C) - U(C \setminus \{m\})
$$

These observations can later train the ContextOS marginal-utility estimator.

## Synthetic benchmark warning

`ctxbench` is only a deterministic policy sanity check. Its success numbers are not evidence of real coding-agent performance.
