# ContextOS — Updated R18.1 Implementation Plan
## Based on the latest retrieval failure analysis

**Date:** 2026-09-25  
**Repository:** `rohitsh16/ContextOS`  
**Primary objective:** Make ContextOS retrieve the correct evidence for natural-language repository questions, then reduce that evidence to minimum sufficient context without sacrificing correctness.

---

## 1. New finding: the immediate problem is wiring, not another retrieval algorithm

The latest investigation identifies **two independent defects**.

### Defect A — R18 HybridRetriever is not on the `ctx plan` execution path

The sophisticated R18 machinery exists:

- `HybridRetriever`
- `RetrieveEvidence`
- query expansion
- hybrid candidate gathering

but it is currently reachable through `ctx retrieve`, while the actual MCP `context_plan` path goes through `Service.Plan`.

The reported path is effectively:

```text
ctx plan
  -> Service.Plan
  -> SearchCandidates
  -> SQLiteStore.SearchCodeCandidates
  -> allocator.Plan
```

while the R18 path is effectively:

```text
ctx retrieve
  -> HybridRetriever
  -> RetrieveEvidence
  -> query expansion
  -> candidate gathering
```

Therefore, improving `HybridRetriever` further will not fix the benchmark used by `ctx plan` until the plan path actually uses it.

### Defect B — legacy retrieval is query-order sensitive

The legacy `SearchCodeCandidates` path:

1. tokenizes the query left-to-right;
2. constructs SQL clauses from those tokens;
3. caps each stage at a small number of slots;
4. consumes the available search budget before the important identifier appears.

For a query such as:

```text
Find where GCP shared-VPC attachment logic excludes the DRMC bunker project...
```

the exact identifier:

```text
gcp_attach_service_project_policy.go
```

may never become an effective retrieval term.

This explains why:

- keyword-dense queries work;
- fluent natural-language paraphrases fail;
- the target file is actually indexed;
- index hygiene is no longer the primary issue.

---

# 2. Revised implementation priority

The next work must be done in this order:

```text
P0  Wire Service.Plan -> R18 HybridRetriever
        |
        v
P1  Preserve deterministic exact identifier/path retrieval
        |
        v
P2  Fix query tokenization / clause budgeting
        |
        v
P3  Add identifier-forward query decomposition
        |
        v
P4  Add bounded semantic/query expansion
        |
        v
P5  Add repository graph expansion
        |
        v
P6  Rerank
        |
        v
P7  Measure Minimum Sufficient Context
        |
        v
P8  Claim verification + answer gate
```

**Do not start with a new embedding model or another sophisticated reranker.**

The current failure can be fixed much earlier in the pipeline.

---

# 3. P0 — Wire `Service.Plan` to the R18 retrieval path

## Objective

Make the retrieval implementation exercised by:

```text
ctx plan
MCP context_plan
integration tests
```

the same retrieval machinery that is already being tested by:

```text
ctx retrieve
```

## Required implementation

Trace the current `Service.Plan` implementation and identify the exact call site that currently invokes:

```text
SearchCandidates
```

Replace or route that candidate-gathering stage through:

```text
RetrieveEvidence / HybridRetriever
```

while preserving the existing plan assembly and output contract.

Target architecture:

```text
Service.Plan
    |
    +--> Query representation
    |
    +--> HybridRetriever
    |       |
    |       +--> exact/path
    |       +--> lexical
    |       +--> semantic
    |       +--> symbol
    |       +--> entity
    |
    +--> repository graph expansion
    |
    +--> ranking
    |
    +--> plan/context bundle
```

### Important

Do not duplicate the retrieval implementation.

There should be one authoritative retrieval path.

`ctx retrieve` and `ctx plan` should call the same core retrieval service.

---

# 4. P0 regression test

Add a test that fails if `ctx plan` silently bypasses R18.

The test should use a query for which:

```text
legacy retrieval fails
R18 retrieval succeeds
```

and assert that:

```text
ctx plan
```

returns the R18 result.

This prevents future architectural regression.

---

# 5. P0 instrumentation

Every `ctx plan` retrieval should expose:

```json
{
  "retrieval_mode": "hybrid",
  "retrieval_stages": [
    "exact_path",
    "lexical",
    "semantic",
    "symbol",
    "graph"
  ],
  "candidate_count": 0,
  "target_rank": 0,
  "target_found": true
}
```

The exact schema can follow the existing ContextOS R18 trace structures.

The key requirement is that the benchmark can prove which path executed.

---

# 6. P1 — Preserve exact identifier/path retrieval

Natural-language robustness must not come at the expense of exact lookup.

Implement a high-priority identifier channel:

```text
path
filename
basename
Go symbol
package
type
method
function
constant
```

For example:

```text
gcp_attach_service_project_policy.go
```

must be recognized as a high-value path/identifier.

Similarly:

```text
isBackupTeam
```

must be recognized as a symbol candidate.

---

# 7. P2 — Fix query tokenization and search-budget starvation

The current legacy behavior is fundamentally unsafe:

```text
query tokens
   -> left-to-right clause creation
   -> each stage receives fixed small budget
   -> important token may appear too late
```

Replace this with query-aware budgeting.

## Required token classes

Classify query tokens as:

```text
PATH
FILENAME
SYMBOL
IDENTIFIER
TECHNICAL_TERM
ENTITY
ACTION
STOPWORD
GENERIC_WORD
```

Example:

```text
Find where GCP shared-VPC attachment logic excludes
the DRMC bunker project gcp_attach_service_project_policy.go
```

should produce approximately:

```text
PATH/FILENAME:
    gcp_attach_service_project_policy.go

ENTITY:
    GCP
    DRMC

TECHNICAL:
    shared-VPC
    attachment
    bunker

ACTION:
    excludes

GENERIC:
    find
    where
    logic
    project
```

The search budget must prioritize the first groups.

---

# 8. Identifier-forward query processing

Before ordinary token retrieval:

```text
1. detect path-like strings
2. detect filename-like strings
3. detect symbol-like strings
4. normalize variants
5. search exact identifiers
6. then execute semantic/lexical retrieval
```

This guarantees that the most discriminative term cannot be lost because of query position.

---

# 9. Filename and symbol variant generation

For:

```text
gcp_attach_service_project_policy.go
```

generate bounded variants:

```text
gcp_attach_service_project_policy
gcp attach service project policy
gcp-attach-service-project-policy
```

For:

```text
isBackupTeam
```

generate:

```text
isBackupTeam
is backup team
is_backup_team
is-backup-team
```

Do not generate unbounded combinations.

Set explicit limits.

---

# 10. Query clause budgeting

Instead of:

```text
first N tokens win
```

use:

```text
identifier budget
technical-term budget
entity budget
semantic budget
generic-term budget
```

Example:

```text
PATH/FILENAME      8
SYMBOL             8
IDENTIFIER         12
TECHNICAL_TERM     16
ENTITY             8
SEMANTIC           bounded
GENERIC             low
```

The exact values must be benchmark-tuned.

---

# 11. Generic stopword handling

Do not simply remove ordinary English words globally.

Instead, downweight terms such as:

```text
find
where
show
tell
what
does
how
logic
code
file
project
thing
```

while preserving technical terms that happen to look common.

The goal is:

```text
less query boilerplate
more discriminative repository vocabulary
```

---

# 12. P3 — Query representation

Create a structured representation:

```go
type QueryRepresentation struct {
    Raw string

    Paths       []string
    Filenames   []string
    Symbols     []string
    Identifiers []string

    Entities  []string
    Concepts  []string
    Actions   []string
    Constraints []string

    Intent QueryIntent
}
```

The representation must be deterministic initially.

Do not make an LLM dependency mandatory for the basic path.

---

# 13. P4 — Hybrid retrieval

After P0–P3 work, use the existing R18 machinery.

The retrieval pipeline should become:

```text
query
  |
  v
query decomposition
  |
  +---- exact path
  +---- filename
  +---- symbol
  +---- lexical
  +---- semantic
  +---- entity
  |
  v
candidate union
  |
  v
repository graph expansion
  |
  v
reranking
```

---

# 14. Candidate recall must be measured before reranking

For every query:

```text
CandidateRecall@100
```

must be calculated before the reranker.

This is critical.

If:

```text
required file not in candidate pool
```

then reranking cannot solve the problem.

---

# 15. P5 — Repository graph expansion

Once the target policy file is found, retrieve its relevant neighborhood.

For the DRMC example:

```text
gcp_attach_service_project_policy.go
        |
        +--> isBackupTeam
        |
        +--> callers
        |
        +--> callees
        |
        +--> related policy
        |
        +--> tests
```

The graph expansion must be bounded and typed.

Prioritize:

```text
direct callers
direct callees
tests
implementations
same-package related files
```

---

# 16. Required DRMC benchmark

The canonical benchmark query should have known required evidence.

Example:

```text
Find where GCP shared-VPC attachment logic excludes the DRMC bunker project.
```

Required evidence should include:

```text
gcp_attach_service_project_policy.go
```

and the relevant `isBackupTeam` call chain identified by the benchmark fixture.

The exact required set must remain in the benchmark manifest rather than being inferred from the implementation under test.

---

# 17. Query robustness benchmark

Create at least:

```text
20 tasks
x
10 formulations/task
=
200 queries
```

For each task:

### Identifier-heavy

```text
gcp_attach_service_project_policy DRMC bunker shared VPC
```

### Natural language

```text
Find where GCP shared-VPC attachment logic excludes the DRMC bunker project.
```

### Short

```text
Where is DRMC shared-VPC exclusion implemented?
```

### Action-oriented

```text
Trace how DRMC bunker projects are prevented from attaching to the shared VPC.
```

### User-like

```text
Why don't these DRMC projects get attached to the VPC?
```

---

# 18. Mandatory ablation sequence

Run exactly:

```text
A0 = current ctx plan
A1 = ctx plan -> HybridRetriever
A2 = A1 + identifier extraction
A3 = A2 + query-budget fix
A4 = A3 + bounded expansion
A5 = A4 + graph expansion
A6 = A5 + reranking
```

Record every stage independently.

This will tell us exactly where the improvement comes from.

---

# 19. Core retrieval metrics

For every ablation:

```text
Recall@1
Recall@5
Recall@10
Recall@20
Recall@50

CandidateRecall@100

MRR
NDCG

RequiredEvidenceRecall
```

For the paraphrase benchmark:

```text
PSI@5
PSI@10
PSI@20
Recall variance across paraphrases
```

---

# 20. New critical benchmark: execution-path correctness

Add:

```text
PlanRetrievalParity
```

Definition:

```text
Plan retrieval result
vs
Direct retrieve result
```

for the same query.

Target:

```text
same authoritative candidate set
```

within the configured ranking tolerance.

This prevents the system from having two materially different retrieval engines.

---

# 21. Negative benchmark

A correct system must not retrieve something merely because it contains related words.

Add questions with no supporting evidence.

Expected:

```text
ABSTAIN
```

or:

```text
NO_EVIDENCE
```

Measure:

```text
false retrieval
false support
false answer
```

---

# 22. Pollution regression

Every retrieval change must still run:

```text
worktree pollution tests
vendor pollution tests
generated-file tests
build-artifact tests
cache tests
```

Required invariant:

```text
inadmissible evidence retrieved = 0
```

This is a hard gate.

---

# 23. R18.1 GREEN criteria

Do not proceed to MSE until:

```text
CandidateRecall@100 >= 99%
Recall@20 >= 98%
RequiredEvidenceRecall >= 95%
PSI@20 >= 0.80
```

on the curated benchmark.

Also require:

```text
0 protected pollution regressions
```

and:

```text
ctx plan uses R18 retrieval
```

as proven by instrumentation.

---

# 24. Only after R18.1 GREEN: Minimum Sufficient Context

Then connect real retrieval candidates to MSE.

Pipeline:

```text
ctx plan
   |
   v
HybridRetriever
   |
   v
Evidence Graph
   |
   v
Candidate Frontier
   |
   v
MSE
   |
   v
LLM
```

The MSE objective is:

```text
minimize context cost
```

subject to:

```text
answer correctness >= threshold
required evidence coverage >= threshold
unsupported claims <= threshold
contradiction risk <= threshold
```

---

# 25. Context ladder experiment

For each task construct:

```text
C1 ⊂ C2 ⊂ C3 ... ⊂ Cn
```

Measure:

```text
answer correctness
claim correctness
unsupported claims
contradictions
tokens
latency
cost
```

The empirical MSE is the smallest context that still satisfies the correctness constraints.

---

# 26. Minimality test

After selecting MSE:

```text
for each evidence unit e:
    remove e
    rerun sufficiency evaluation
```

If the answer remains sufficiently correct:

```text
e was redundant
```

Therefore the original context was not minimal.

Report:

```text
minimality rate
redundancy rate
```

---

# 27. Correctness architecture after MSE

Final target:

```text
USER QUERY
    |
    v
QUERY UNDERSTANDING
    |
    v
HYBRID RETRIEVAL
    |
    v
EVIDENCE GRAPH
    |
    v
MINIMUM SUFFICIENT CONTEXT
    |
    v
LLM
    |
    v
CLAIM EXTRACTION
    |
    v
CLAIM VERIFICATION
    |
    +---- supported ------> ANSWER
    |
    +---- missing --------> RETRIEVE MORE
    |
    +---- contradiction --> INVESTIGATE
    |
    +---- unsupported ----> ABSTAIN
```

---

# 28. What NOT to do now

Do not:

- add another exclusion rule;
- change the index because the current evidence shows the target is indexed;
- add more stopwords as the primary solution;
- immediately introduce a more expensive LLM reranker;
- increase top-K indefinitely;
- optimize MSE before candidate recall is fixed;
- treat `ctx retrieve` success as proof that `ctx plan` is fixed.

The current evidence specifically indicates that the **execution path used by `ctx plan` is the first thing to repair**.

---

# 29. Immediate implementation checklist

## P0 — must happen first

```text
[ ] Locate Service.Plan
[ ] Trace current SearchCandidates call
[ ] Route Service.Plan through HybridRetriever
[ ] Preserve Plan output contract
[ ] Add retrieval-mode telemetry
[ ] Add Plan-vs-Retrieve parity test
[ ] Re-run DRMC benchmark
```

## P1

```text
[ ] Extract paths
[ ] Extract filenames
[ ] Extract symbols
[ ] Extract identifiers
[ ] Prioritize identifier channel
```

## P2

```text
[ ] Replace left-to-right query budget
[ ] Add token classes
[ ] Add identifier-first search
[ ] Add bounded variants
[ ] Downweight generic query words
```

## P3+

```text
[ ] Query representation
[ ] Hybrid retrieval
[ ] Candidate recall metrics
[ ] Graph expansion
[ ] Reranking
```

---

# 30. The most important immediate experiment

Before implementing anything broad, run this controlled comparison:

```text
Query:
"Find where GCP shared-VPC attachment logic excludes the DRMC bunker project."

Run:

1. ctx plan — current path
2. ctx retrieve — current R18 path
3. ctx plan — after wiring to R18
4. ctx plan — R18 + identifier extraction
5. ctx plan — R18 + query-budget fix
```

For each run capture:

```text
target found?
target rank?
candidate count?
candidate recall?
required evidence recall?
top 20 paths?
retrieval stages?
latency?
input tokens?
```

The expected scientific outcome is:

```text
1. current ctx plan -> FAIL
2. ctx retrieve -> substantially better
3. plan -> R18 -> closes architectural gap
4. identifier extraction -> improves robustness
5. query-budget fix -> removes phrase-order sensitivity
```

If step 3 does not reproduce step 2, stop and debug the integration before adding any new retrieval algorithm.

---

# 31. Final research direction

The latest evidence actually makes the research problem cleaner.

We are no longer primarily fighting:

```text
"ContextOS indexes the wrong files."
```

The immediate problem is:

```text
"ContextOS has better retrieval machinery, but the production plan path
does not consistently use it, and the legacy path is sensitive to query
token ordering."
```

After fixing that, the genuine research problem becomes:

> Can ContextOS transform an arbitrary natural-language software-engineering task into a provenance-aware, query-robust, minimum sufficient evidence set, and use that evidence to produce a correct answer with calibrated abstention?

That is the problem to benchmark rigorously.

The implementation order remains:

```text
WIRE
  ↓
IDENTIFIER-AWARE RETRIEVAL
  ↓
QUERY-ROBUST RETRIEVAL
  ↓
GRAPH COMPLETENESS
  ↓
MINIMUM SUFFICIENT CONTEXT
  ↓
CLAIM VERIFICATION
  ↓
ANSWER / RETRIEVE MORE / ABSTAIN
```
