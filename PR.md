# ContextOS — Research-Grade Implementation Plan

## 0. Objective

Transform the current ContextOS implementation into a reproducible research system for persistent engineering context management across AI coding agents.

The implementation should ultimately optimize:

$$
C^*
=
\arg\max_{C\subseteq \mathcal{M}}
U(C)
$$

subject to:

$$
Tokens(C)\le B
$$

and, where task-success supervision is available,

$$
P(\mathrm{Success}\mid C,q,S,M)\ge \tau
$$

where:

* \(C\) = selected context
* \(\mathcal{M}\) = candidate memory/context pool
* \(q\) = current task/query
* \(S\) = repository/session state
* \(M\) = model/agent
* \(B\) = token budget
* \(U(C)\) = context utility

Use a more complete utility decomposition:

$$
U(C)=
Q(C)
+\eta R_{\text{cache}}(C)
+\gamma R_{\text{graph}}(C)
+\delta R_{\text{coverage}}(C)
-\lambda Cost(C)
-\mu Latency(C)
-\rho Risk_{\text{stale}}(C)
-\xi Redundancy(C)
$$

The implementation should progressively replace hand-tuned approximations with measured or learned terms.

---

# 1. Current Baseline

The current live implementation should be frozen as the experimental baseline.

Record:

```text
BASELINE_SHA=<current main commit SHA>
BASELINE_VERSION=0.6.0
BASELINE_DATE=<date>
GO_VERSION=<version>
OS=<platform>
CPU=<hardware>
RAM=<hardware>
```

Do not compare "old code" against "new code" using different repositories, machines, task sets, or budgets.

All A/B experiments must use:

```text
same repository snapshot
same task set
same model
same model parameters
same token budget
same agent/tool settings
same random seeds where applicable
same timeout
same machine
```

The current `BENCHMARK_REPORT.md` numbers are engineering telemetry, not yet a scientific evaluation. The current sample is only 16 real-world allocation traces, with approximately 50.5% token reduction and 31.3% cache hits. Those figures are useful for regression monitoring, but they do not establish improved agent task success.

Treat the existing numbers as:

$$
\text{engineering telemetry}
\neq
\text{scientific evidence}
$$

---

# 2. Global Benchmark Framework

Before implementing the PRs, create a standard benchmark schema.

Every benchmark result should contain:

```json
{
  "commit_sha": "...",
  "version": "...",
  "benchmark_id": "...",
  "task_id": "...",
  "repo_id": "...",
  "repo_commit": "...",
  "worktree_fingerprint": "...",
  "model": "...",
  "budget": 8192,
  "retrieval_policy": "...",
  "allocator_policy": "...",
  "context_tokens": 0,
  "latency_ms": 0,
  "cache_prefix_tokens": 0,
  "cache_hit": false,
  "task_success": null,
  "regression": null,
  "memory_recall": null,
  "evidence_recall": null
}
```

Never mix:

```text
retrieval quality
context compression
agent task success
provider cache hit
local context-plan cache hit
```

They are different variables.

---

# 3. PR-01 — Portable Integration Configuration

## Goal

Remove machine-local paths and make repository integrations reproducible across fresh clones.

### Current problem

Committed integration files contain absolute paths such as:

```text
/Users/rohitshukla/Desktop/ContextOS/bin/ctx-hook
/Users/rohitshukla/Desktop/ContextOS/bin/contextd
```

This makes a fresh clone inherit the original developer's filesystem layout.

The desired invariant is:

$$
\text{Clone}(R,m_1)
\Rightarrow
\text{Clone}(R,m_2)
$$

must produce functionally equivalent configuration after installation.

## Implementation

### Step 1

Identify generated files:

```text
.cursor/hooks.json
.cursor/mcp.json
.agents/hooks.json
.agents/mcp_config.json
```

and determine which should be:

```text
tracked templates
```

versus:

```text
generated local configuration
```

Prefer generating user-specific files during:

```bash
ctx setup
```

rather than committing them.

### Step 2

Add a portable executable resolution strategy.

Preferred order:

```text
explicit CONTEXTOS_BIN
        ↓
PATH lookup
        ↓
repo-local ./bin/
        ↓
well-defined installation path
```

Avoid embedding developer-specific paths.

### Step 3

Make installation idempotent.

Required invariant:

$$
Install(Install(R)) = Install(R)
$$

where the second installation should not duplicate hooks, MCP entries, or rules.

### Step 4

Add generated-config markers.

For example:

```json
{
  "_managedBy": "contextos",
  "_version": "0.7.x"
}
```

This allows the installer to distinguish:

```text
ContextOS-managed configuration
```

from:

```text
user-owned configuration
```

### Step 5

Make merge operations structurally aware.

Do not blindly append JSON arrays.

Use stable identity:

```text
hook.command
mcp.server.name
rule.name
```

to detect an existing installation.

## Tests

### Unit

Test:

```text
fresh install
second install
partial existing config
existing unrelated hook
existing unrelated MCP server
different binary path
relative binary path
PATH-resolved binary
missing binary
```

### Property test

Generate arbitrary pre-existing configurations and verify:

$$
Install(C)
$$

preserves unrelated fields.

### Golden test

For every supported provider:

```text
Claude
Cursor
Codex
Gemini
Antigravity
```

compare generated configuration against golden fixtures.

## Benchmark

Measure:

```text
setup_time_ms
config_files_created
duplicate_entries
failed_integrations
fresh_clone_success_rate
```

Target:

```text
100% successful fresh clones
0 duplicated ContextOS entries
0 absolute user-home paths
```

---

# 4. PR-02 — Cryptographically Correct Worktree Fingerprinting

## Goal

Make context cache invalidation correct.

### Current defect

The current dirty-worktree fingerprint hashes Git status text.

That means:

```text
file.go modified
```

can produce the same fingerprint regardless of the actual contents of `file.go`.

Therefore:

$$
WT(A)=WT(B)
$$

can hold even though:

$$
A\neq B
$$

This can cause incorrect context-plan reuse.

## New design

Use a content-addressed Merkle-style fingerprint.

For each relevant file:

$$
h_f =
H(
path
\Vert
status
\Vert
content
)
$$

Then compute:

$$
H_{WT}
=
H(
h_1\Vert h_2\Vert \dots \Vert h_n
)
$$

Sort files lexicographically before hashing so the fingerprint is order independent.

### Exclusions

Exclude:

```text
.git/
.contextos/
node_modules/
vendor/
build/
dist/
target/
large generated files
```

but make exclusions explicit in configuration.

## Advanced improvement: Merkle hierarchy

Instead of:

$$
O(N)
$$

hashing every file on every request, use:

```text
repo
 ├── src
 │    ├── auth
 │    └── payment
 ├── tests
 └── docs
```

and compute directory hashes:

$$
H_d=H(H_{child_1}\Vert\dots\Vert H_{child_k})
$$

Then a localized change invalidates only the affected ancestry path.

This becomes the foundation for incremental indexing and cache correctness.

## Tests

Create temporary Git repos.

Test:

```text
clean repo → fingerprint F1
edit tracked file → F2
edit different tracked file → F3
restore original content → F1
add untracked file → F4
change untracked contents → F5
delete file → F6
modify ignored file → unchanged
change .contextos → unchanged
```

Critical invariant:

$$
Content(A)\neq Content(B)
\Rightarrow
Fingerprint(A)\neq Fingerprint(B)
$$

for all tracked/untracked relevant files.

## Collision test

Use thousands of generated mutations.

Verify zero observed collisions.

This is empirical evidence only; do not call this a cryptographic proof.

## Benchmark

Measure:

```text
fingerprint latency
cold fingerprint latency
warm fingerprint latency
number of files read
bytes read
cache hit rate
false cache reuse rate
```

Compare:

```text
v0.6.0 status-hash
new content-addressed fingerprint
```

The most important metric:

$$
FalseReuseRate
$$

Target:

```text
0 observed false cache reuses
```

---

# 5. PR-03 — Replace Graph Proxy with Actual Graph Intelligence

## Goal

Replace path-string heuristics with graph-derived structural relevance.

### Current problem

The current "graph score" is essentially:

```text
path separators
extension markers
kind heuristics
```

That is not graph centrality.

Meanwhile ContextOS already builds reference edges.

Use those edges.

---

# Graph Model

Define:

$$
G=(V,E,W)
$$

where:

* \(V\) = code symbols/files
* \(E\) = imports, calls, type references, tests, configuration references
* \(W\) = edge weights

Different edges should have different weights:

```text
call           1.0
import         0.8
type-reference 0.6
test-reference 0.9
documentation  0.3
```

Keep weights configurable.

---

# Task-conditioned Personalized PageRank

Rather than global centrality, compute:

$$
r
=
\alpha s
+
(1-\alpha)P^Tr
$$

where:

* \(s\) = task-local seed vector
* \(P\) = normalized transition matrix
* \(\alpha\) = restart probability

This gives:

```text
global importance
```

an additional task-conditioned component:

```text
importance relative to current task
```

Personalized PageRank is attractive because it naturally diffuses relevance over dependency structure and has scalable approximate variants.

## Seed construction

Seeds can come from:

```text
exact identifier matches
memory locations
changed Git files
test failures
stack traces
recently edited files
retrieved documents
explicit user targets
```

Define:

$$
s(v)=
w_q q(v)
+w_d d(v)
+w_f f(v)
+w_t t(v)
$$

Normalize:

$$
\sum_v s(v)=1
$$

---

# Advanced graph score

Use:

$$
GScore(v)
=
\beta_1 PPR(v)
+\beta_2 Degree(v)
+\beta_3 TestCentrality(v)
+\beta_4 ChangeProximity(v)
+\beta_5 DependencyBridge(v)
$$

Do not use raw degree directly because high-degree utility files otherwise dominate everything.

Consider:

$$
DegreeNorm(v)
=
\frac{\log(1+deg(v))}
{\log(1+deg_{\max})}
$$

---

# Approximation

Do not run full matrix inversion.

Use approximate push/PPR algorithms.

The implementation goal:

$$
O((|E|+|V|)\cdot \epsilon^{-1})
$$

or better in practice through sparse localized computation.

Only compute relevance for the task neighborhood.

## Tests

Construct known graphs:

```text
chain
star
diamond
hub-and-spoke
dependency tree
test dependency graph
```

Verify expected ranking.

Example:

```text
task -> A -> B -> C
             \
              D
```

A should rank highest, followed by B, then C/D depending on weights.

Test disconnected nodes.

Test cyclic graphs.

Test identical-text but structurally different files.

## Benchmark

Compare:

```text
path heuristic
degree
PageRank
personalized PageRank
PPR + lexical
PPR + lexical + semantic
```

Metrics:

```text
MRR
Recall@K
nDCG@K
task-relevant-file recall
candidate generation latency
```

A useful research metric:

$$
GraphLift@K
=
Recall_{graph}@K
-
Recall_{text}@K
$$

---

# 6. PR-04 — Incremental Indexing

## Goal

Move from:

$$
O(N)
$$

full repository rebuilding toward:

$$
O(\Delta + affected\_closure)
$$

incremental indexing.

## Current problem

`SaveNodesAndEdges()` clears and rebuilds the repository index.

For large repositories:

```text
one file changed
→ entire repo rescanned
→ entire graph rewritten
```

---

# File Manifest

Create a persistent index manifest:

```text
path
content_hash
size
mtime
language
symbol_hash
dependency_hash
indexed_revision
```

### Change detector

For every indexing operation:

$$
Changed
=
Added
\cup Deleted
\cup Modified
\cup DependencyAffected
$$

### Symbol-level hashing

For each file:

$$
H_{symbol}
=
H(
symbol_1
\Vert
...
\Vert
symbol_n
)
$$

This allows distinguishing:

```text
file changed
```

from:

```text
relevant symbol changed
```

---

# Dependency-aware invalidation

Suppose:

```text
A imports B
B changed
```

Then potentially:

$$
Affected(B)=
\{B\}\cup
Ancestors(B)
$$

Do not rebuild the entire graph.

Use reverse dependency edges to calculate affected closure.

---

# Advanced optimization

Use a two-tier invalidation:

### Tier 1

Cheap file-level hash detection.

### Tier 2

Only changed files undergo symbol parsing.

### Tier 3

Only affected graph neighborhoods undergo graph recalculation.

Thus:

$$
T_{index}
\approx
T_{hash}
+
T_{\Delta}
+
T_{closure}
$$

instead of:

$$
T_{full}
=
T_{scan}+T_{parse}+T_{graph}
$$

---

# Tests

Create repositories from:

```text
10 files
100 files
1k files
10k files
```

Modify:

```text
one leaf
one hub
one dependency
rename file
delete file
add file
change unrelated documentation
```

Verify:

```text
only necessary files reindexed
deleted nodes disappear
edges remain consistent
no duplicate nodes
no stale edges
```

Consistency property:

$$
IndexIncremental(R,\Delta)
\equiv
IndexFull(R+\Delta)
$$

Compare both resulting graphs.

This equivalence test is one of the most important tests in the project.

## Benchmark

Measure:

```text
full index time
incremental index time
files parsed
symbols parsed
edges rewritten
DB writes
CPU
memory
```

Report:

$$
Speedup =
\frac{T_{full}}{T_{incremental}}
$$

and:

$$
WorkReduction =
1-\frac{Files_{\Delta}}{Files_{full}}
$$

---

# 7. PR-05 — True Corpus-Aware BM25

## Goal

Replace BM25-style scoring with actual corpus-aware BM25.

The current implementation lacks meaningful corpus-level document-frequency statistics.

---

# Mathematical model

For term \(t\):

$$
IDF(t)
=
\ln
\left(
1+
\frac{N-df(t)+0.5}
{df(t)+0.5}
\right)
$$

Then:

$$
BM25(D,Q)
=
\sum_{t\in Q}
IDF(t)
\frac{
tf_{t,D}(k_1+1)
}{
tf_{t,D}
+
k_1
\left(
1-b+b\frac{|D|}{avgdl}
\right)
}
$$

Use:

```text
k1 ≈ 1.2–2.0
b  ≈ 0.75
```

but tune only through benchmark data.

Do not hard-code the final values based on intuition.

---

# Candidate-set statistics

Because ContextOS already works on a task-local candidate pool, calculate:

```text
N
df(term)
avgdl
```

over the candidate corpus or a defined repository corpus.

Compare both:

```text
global corpus IDF
task candidate IDF
repository IDF
```

This itself becomes an experiment.

---

# Advanced extension: BM25+

Evaluate BM25+ and BM25L against baseline BM25 where long-document normalization could distort code/document relevance.

Do not automatically replace BM25 with BM25+.

Create a benchmark matrix.

---

# Tests

Known corpus:

```text
D1: kafka transactional outbox
D2: kafka retries
D3: postgres transaction
D4: unrelated
```

Verify rare terms dominate frequent terms.

Test:

```text
short doc
long doc
repeated term
rare identifier
common identifier
empty query
empty corpus
```

## Benchmark

Compare:

```text
lexical overlap
current BM25-style
true BM25
BM25+
BM25L
```

Metrics:

```text
MRR@10
Recall@10
nDCG@10
P@1
latency
CPU
index size
```

Repository-level code search research shows that commit-history-aware and neural reranking can substantially outperform a BM25 baseline, which reinforces the need to keep lexical retrieval as a strong baseline rather than assuming it is sufficient by itself.

---

# 8. PR-06 — Real Semantic Retrieval

## Goal

Replace the current hash-feature "semantic-ish" similarity with a true pluggable semantic retrieval layer.

The existing hashing approach remains valuable as a zero-dependency baseline.

Call it:

```text
FeatureHashSimilarity
```

not:

```text
semantic embedding
```

---

# Architecture

Define:

```go
type EmbeddingProvider interface {
    Embed(ctx context.Context, texts []string) ([][]float32, error)
}
```

Implement:

```text
HashEmbeddingProvider
LocalEmbeddingProvider
HTTPEmbeddingProvider
```

The default should remain dependency-light.

---

# Retrieval tiers

Use:

```text
Stage 1: lexical retrieval
Stage 2: graph retrieval
Stage 3: dense retrieval
Stage 4: fusion
Stage 5: context selection
```

Do not run expensive semantic models over the whole repository on every query.

---

# Hybrid Retrieval

Let:

$$
R_L(d)
$$

be lexical rank,

$$
R_G(d)
$$

graph rank, and

$$
R_D(d)
$$

dense rank.

Combine using RRF:

$$
RRF(d)
=
\sum_i
\frac{1}{k+rank_i(d)}
$$

Then optionally learn:

$$
Score(d)=
w_L R_L(d)+
w_G R_G(d)+
w_D R_D(d)
$$

---

# Advanced research direction: late interaction

For large repositories, evaluate a ColBERT-style architecture:

$$
Score(Q,D)
=
\sum_{q_i\in Q}
\max_{d_j\in D}
sim(q_i,d_j)
$$

This preserves token-level matching instead of compressing the entire document into one vector.

ColBERT showed the usefulness of late interaction, and ColBERTv2 introduced compression techniques that greatly reduce representation footprint.

Do this as an optional research backend, not as a mandatory dependency.

---

# Sparse neural alternative

Evaluate SPLADE-like sparse representations as another research backend because they retain sparse lexical structure while adding neural expansion.

The research matrix becomes:

```text
BM25
HashSemantic
Dense
BM25 + Dense
BM25 + Graph + Dense
BM25 + Graph + LateInteraction
SparseNeural + Graph
```

---

# Tests

Tests should assert:

```text
provider failure → lexical fallback
dimension mismatch → error
empty embedding → fallback
duplicate embedding → valid
batch vs single embedding → equivalent within tolerance
```

## Benchmark

Measure:

```text
Recall@K
MRR
nDCG
query latency
embedding generation cost
index size
memory usage
task-success impact
```

Most important:

$$
\Delta Success / \Delta Tokens
$$

not merely retrieval accuracy.

---

# 9. PR-07 — Structured Memory Extraction

## Goal

Replace phrase-matching heuristics with a structured memory pipeline.

Current approach recognizes strings like:

```text
we decided
decision:
use outbox
use kafka
```

This does not scale.

---

# New architecture

Use:

```text
Raw Event
    ↓
Event Normalization
    ↓
Candidate Fact Extraction
    ↓
Evidence Binding
    ↓
Confidence Calibration
    ↓
Conflict Detection
    ↓
Persistence
```

Memory should become a claim with evidence, not merely text.

---

# Memory representation

Conceptually:

$$
m=
(
claim,
type,
scope,
authority,
timestamp,
validity,
evidence,
confidence,
dependencies
)
$$

Example:

```json
{
  "claim": "The retry path uses transactional outbox",
  "kind": "decision",
  "scope": "repository",
  "authority": 0.94,
  "confidence": 0.91,
  "locations": ["internal/outbox/..."],
  "evidence": [
    {
      "type": "commit",
      "id": "abc123"
    },
    {
      "type": "source",
      "path": "..."
    }
  ]
}
```

---

# Provenance graph

Build:

$$
Evidence
\rightarrow
Claim
\rightarrow
DerivedMemory
$$

This creates a provenance DAG.

A memory without evidence should have lower confidence.

---

# Confidence

Instead of treating confidence as a hand-written scalar, separate:

$$
Authority(m)
$$

from:

$$
Confidence(m)
$$

from:

$$
Freshness(m)
$$

and:

$$
EvidenceStrength(m)
$$

Then:

$$
P(\text{claim valid})
\approx
f(
Authority,
Confidence,
Freshness,
EvidenceStrength
)
$$

Do not multiply these probabilities naively.

They are not necessarily statistically independent.

---

# Advanced research: calibrated confidence

Use held-out historical data to calibrate memory confidence.

Potential techniques:

```text
isotonic regression
Platt scaling
conformal calibration
```

Conformal risk-control methods are particularly interesting because they can provide explicit risk guarantees for selective prediction/rejection instead of relying on raw model confidence.

---

# Conflict resolution

Represent conflicting claims:

$$
m_1 \neq m_2
$$

instead of silently overwriting.

Rank evidence:

```text
verified source
test
commit
explicit user decision
agent inference
```

Then compute a support relation.

Potentially model support as:

$$
Support(m)
=
\sum_i w_i e_i
$$

but preserve the individual evidence rather than only the scalar.

---

# Tests

Create traces containing:

```text
decision
reversal
contradiction
temporary experiment
successful fix
failed fix
explicit user correction
```

Verify:

```text
decisions extracted
evidence preserved
contradictions preserved
obsolete claims detected
duplicate claims merged
```

Critical test:

```text
Agent says A
later says B
B must not silently mutate the historical record of A.
```

Instead:

```text
A valid [t0,t1]
B valid [t1,...]
```

---

# Benchmark

Metrics:

```text
Memory Precision
Memory Recall
Duplicate Rate
Conflict Detection
Evidence Attachment Rate
Calibration Error
Useful-Memory@K
```

Calibration metrics:

```text
ECE
Brier score
selective risk
coverage vs error
```

---

# 10. PR-08 — Temporal Consistency and Scoped Validity

## Goal

Stop treating Git revision mismatch as a crude freshness penalty.

Current logic is effectively:

```text
revision changed
→ memory somewhat stale
```

That is insufficient.

A memory about:

```text
README.md
```

does not necessarily become invalid because:

```text
payment_service.go
```

changed.

---

# Version-aware validity model

Give each memory a validity interval:

$$
m:[r_{start},r_{end}]
$$

and scope:

$$
Scope(m)
\subseteq
Repository
$$

Possible scope forms:

```text
repository
directory
file
symbol
test
work-item
branch
```

---

# Git DAG reasoning

Let:

$$
G_{git}=(V,E)
$$

be the commit DAG.

For a memory created at revision \(r_m\) and current revision \(r_c\), find:

$$
Diff(r_m,r_c)
$$

Then calculate whether the memory's dependency scope intersects changed paths.

Validity becomes:

$$
Valid(m,r_c)=
Ancestor(r_m,r_c)
\land
NoRelevantChange(m,r_m,r_c)
$$

---

# Scoped staleness score

Define:

$$
StaleRisk(m)
=
1-
P(
Scope(m)
\text{ unchanged}
\mid
Diff
)
$$

A practical deterministic version:

$$
StaleRisk(m)
=
1-\prod_{f\in Scope(m)}
(1-p_f)
$$

where \(p_f\) represents evidence that file \(f\) has materially changed.

---

# Dependency-aware invalidation

If memory refers to:

```text
A
```

and:

```text
A -> B -> C
```

then changes to B/C may matter depending on claim type.

Distinguish:

```text
textual dependency
semantic dependency
runtime dependency
test dependency
```

Do not invalidate everything simply because the graph is connected.

---

# Advanced statistical extension

Treat memory usefulness as a time-to-event process.

Define:

$$
T_m
=
\text{time/revisions until memory becomes invalid}
$$

Estimate a survival curve:

$$
S_m(t)=P(T_m>t)
$$

Then learn a hazard:

$$
h_m(t)
=
\lim_{\Delta t\to0}
\frac{
P(t\le T_m<t+\Delta t\mid T_m\ge t)
}{\Delta t}
$$

This enables smarter freshness decay.

A configuration decision may have a long useful lifetime; an incident observation may decay quickly.

Concept-drift research provides a useful framework for thinking about changing distributions and validity over time rather than assuming stationarity.

---

# Tests

Test:

```text
new commit unrelated to memory
new commit modifying memory file
dependency changed
test changed
branch diverged
merge
rebase
force replacement
uncommitted modification
file rename
file delete
```

Required property:

```text
unrelated changes do not unnecessarily invalidate scoped memories
```

and:

```text
relevant changes cannot leave stale memories falsely fresh
```

---

# Benchmark

Compare:

```text
revision-only penalty
file-scoped validity
dependency-scoped validity
survival-based freshness
```

Metrics:

```text
FalseFreshRate
FalseStaleRate
Memory usefulness
retrieval precision
task success
```

The most important metric is:

$$
TemporalRisk
=
FalseFreshRate + FalseStaleRate
$$

---

# 11. PR-09 — MCP Protocol Correctness and Portability

## Goal

Bring ContextOS MCP implementation into compliance with the current protocol rather than implementing a hybrid of older and current behavior.

The MCP specification dated 2026-07-28 is materially different from earlier revisions: the protocol is stateless, the old `initialize` handshake was removed, every request carries protocol metadata, and `server/discover` is now mandatory.

The tools capability also requires deterministic tool listing and supports pagination/caching semantics.

---

# Implementation

### Step 1

Implement:

```text
server/discover
```

with:

```text
supportedVersions
capabilities
serverInfo
instructions
ttl
cache scope
```

### Step 2

Support request `_meta`:

```text
io.modelcontextprotocol/protocolVersion
io.modelcontextprotocol/clientInfo
io.modelcontextprotocol/clientCapabilities
```

### Step 3

Preserve backward compatibility where useful.

Architect:

```text
modern protocol layer
        ↓
compatibility adapter
        ↓
legacy initialize clients
```

Do not mix protocols throughout application logic.

### Step 4

Implement deterministic ordering for:

```text
tools/list
resources/list
```

This is not cosmetic: deterministic ordering helps downstream caching.

### Step 5

Implement pagination state correctly.

### Step 6

Remove:

```text
hardcoded /Users/rohitshukla/.../mcp_debug.log
```

Use:

```text
CONTEXTOS_DEBUG_LOG
```

or:

```text
~/.contextos/logs/
```

### Step 7

Debug logging should be opt-in.

Default:

```text
disabled
```

---

# Tests

Protocol conformance fixtures:

```text
server/discover
tools/list
tools/call
resources/list
invalid request
unknown method
notification
protocol mismatch
pagination
determinism
```

Run the same request multiple times:

$$
tools/list_1 = tools/list_2
$$

when the underlying tool set is unchanged.

---

# Benchmark

Measure:

```text
MCP startup
discover latency
tools/list latency
resource/list latency
memory footprint
JSON serialization cost
request throughput
```

Also perform compatibility tests with each supported agent.

---

# 12. PR-10 — Research Integrity and Mathematical Claims

## Goal

Make the repository publication-grade in terms of what is claimed versus what is actually implemented.

This is a critical PR.

---

# Changes

### Remove unsupported statement

Do not claim:

$$
1-\frac1e
$$

for the current heuristic.

Instead say:

```text
density-greedy selection with singleton rescue and fill pass
```

until the implementation is upgraded and a formal proof applies.

Sviridenko's \(1-1/e\) result applies to a specific monotone submodular knapsack setting and algorithmic construction, not merely any greedy-plus-singleton allocator.

### Add explicit assumptions

For any future guarantee state:

```text
Assumption A1: monotonicity
Assumption A2: submodularity
Assumption A3: non-negative costs
Assumption A4: feasibility
...
```

Then prove:

```text
Lemma
Theorem
Corollary
```

rather than embedding theoretical claims in comments.

---

# Introduce a formal objective

Define:

$$
F(C)=
\sum_i
\alpha_i
Coverage_i(C)
+
\sum_j
\beta_j
Relevance_j(C)
-
\lambda Redundancy(C)
+
\eta Cache(C)
-
\rho Risk(C)
$$

Then explicitly determine which terms are:

```text
modular
submodular
supermodular
non-monotone
heuristic
learned
```

This classification is extremely important.

---

# Research-grade optimizer path

Create an abstraction:

```go
type Selector interface {
    Select(ctx Context, candidates []Candidate, budget int) ([]Candidate, Diagnostics)
}
```

Implement:

```text
GreedySelector
GreedySingletonSelector
SubmodularSelector
LazyGreedySelector
LearnedSelector
```

This makes theoretical comparisons possible.

---

# 13. PR-11 — Scientific Benchmarking

## Goal

Convert ContextOS benchmarking from engineering telemetry into controlled experiments.

---

# Benchmark layers

## Layer A — Retrieval

Measures:

```text
Recall@K
MRR
nDCG
MAP
```

## Layer B — Selection

Measures:

```text
token usage
coverage
redundancy
utility
budget utilization
```

## Layer C — Agent outcome

Measures:

```text
task success
test pass
regression
handoff success
rediscovery
```

## Layer D — Economics

Measures:

```text
input tokens
cached tokens
uncached tokens
output tokens
latency
estimated cost
```

Do not infer provider cache savings solely from ContextOS's own plan-cache hits. Provider prompt caches depend on actual repeated prompt prefixes; for example, Claude documents cache keys around identical prompt prefixes and explicit/automatic cache breakpoints.

---

# Experimental baselines

At minimum:

```text
B0 Full history
B1 Recency
B2 Top-K lexical
B3 Current ContextOS
B4 BM25
B5 BM25 + dense
B6 BM25 + graph
B7 Hybrid retrieval
B8 Hybrid + submodular selection
B9 Full ContextOS research system
```

For agent benchmarks also include:

```text
persistent memory OFF
persistent memory ON
graph OFF
graph ON
cache-aware OFF
cache-aware ON
temporal validity OFF
temporal validity ON
```

---

# Budget sweep

Run:

```text
512
1,024
2,048
4,096
8,192
16,384
32,768
```

For each budget measure:

$$
Success(B)
$$

and:

$$
Cost(B)
$$

This gives a quality-cost frontier.

---

# Statistical analysis

Do not compare means alone.

Use paired measurements because every method should run on the same task.

For each task:

$$
\Delta_i
=
Metric_{new,i}
-
Metric_{baseline,i}
$$

Report:

```text
mean Δ
median Δ
95% bootstrap CI
```

Also report:

```text
paired permutation test
Cliff's delta
```

For multiple hypotheses, use:

```text
Holm-Bonferroni correction
```

rather than treating every p-value independently.

---

# Power analysis

Before the full benchmark, estimate required \(n\).

For success-rate comparisons, determine detectable effect size:

$$
\delta_{min}
$$

with:

```text
alpha = 0.05
power = 0.80 or 0.90
```

Do not stop an experiment after five successful tasks and call it a result.

---

# Pareto analysis

For every method calculate:

$$
(Tokens,Latency,Success,Cost)
$$

A method is dominated when another method is:

```text
no worse in every dimension
and strictly better in at least one.
```

The final benchmark should report the Pareto frontier rather than one arbitrary winner.

---

# Confidence intervals

For task-level metrics, use bootstrap resampling.

For success probabilities:

$$
\hat p=\frac{k}{n}
$$

report an appropriate binomial confidence interval, preferably Wilson or exact rather than a normal approximation for small \(n\).

---

# 14. PR-12 — Longitudinal Adaptive Context Benchmark

## Goal

This is where ContextOS becomes a genuine research system rather than a sophisticated retriever.

The hypothesis is:

> Persistent engineering context should become more useful over repeated work on the same repository.

This cannot be evaluated with isolated queries.

---

# Longitudinal protocol

For repository \(R\):

```text
Agent A
  ↓
task
  ↓
context retrieval
  ↓
solution
  ↓
memory persistence
  ↓
repository changes
  ↓
branch / restart
  ↓
Agent B
  ↓
same or related task
```

Run multiple generations:

$$
A_1\rightarrow A_2\rightarrow\dots\rightarrow A_T
$$

---

# Metrics

### Rediscovery

How often does the later agent rediscover an already-established fact?

$$
RediscoveryRate
=
\frac{\text{redundantly rediscovered facts}}
{\text{retrievable known facts}}
$$

### Handoff success

$$
HandoffSuccess
=
P(
A_{t+1}\text{ continues correctly}
\mid
Memory(A_t)
)
$$

### Memory usefulness

$$
U_m
=
\frac{
Success(m\ included)-Success(m\ excluded)
}{
Cost(m)
}
$$

### Memory half-life

Estimate revision/time at which utility falls below:

$$
U_m(t) < \theta
$$

This directly connects to the survival model from PR-08.

---

# 15. Advanced Selector Research Track

After PR-05 to PR-08 are stable, evolve the allocator mathematically.

## 15.1 Facility-location context objective

Define:

$$
F(C)
=
\sum_{u\in U}
\max_{c\in C}
sim(u,c)
$$

This rewards representative coverage.

Facility location is a classic monotone-submodular objective, and can be approximated using sparse similarity structures instead of a full \(O(n^2)\) matrix.

Use it to reduce:

```text
duplicate memories
duplicate tool outputs
multiple nearly identical code files
```

---

# 15.2 Query-conditioned mutual information

Model:

$$
F(C;Q)
$$

so selection favors information about the current task rather than generic diversity.

A facility-location mutual-information formulation is one useful construction.

---

# 15.3 Curvature-aware selection

For a submodular function define curvature:

$$
\kappa
=
1-
\min_i
\frac{
F(V)-F(V\setminus i)
}{
F(\{i\})
}
$$

When curvature is small, greedy methods can obtain stronger bounds than worst-case general submodular guarantees.

Use empirical curvature estimates to decide:

```text
simple greedy
```

versus:

```text
more expensive optimizer
```

This creates an adaptive compute policy.

---

# 15.4 Weak submodularity

The real ContextOS utility may not be perfectly submodular because:

```text
cache bonuses
interaction effects
temporal risk
graph dependencies
model-specific behavior
```

may violate classical assumptions.

Instead investigate weak-submodularity or approximate diminishing returns.

Define an empirical ratio:

$$
\gamma
=
\inf_{A\subseteq B,\;i\notin B}
\frac{
\Delta(i\mid B)
}{
\Delta(i\mid A)
}
$$

When:

$$
\gamma < 1
$$

the system is not perfectly submodular, but a useful degree of diminishing returns may still exist.

Recent work on weak-submodular/weak-monotone knapsack problems provides a mathematical framework for precisely this kind of imperfect objective.

This is potentially much more relevant to ContextOS than simply claiming classical submodularity.

---

# 15.5 Adaptive submodularity

Context selection is sequential.

You do not always know whether an item will be useful until other context is selected or examined.

That motivates:

$$
\Delta(i\mid \psi)
$$

where \(\psi\) represents observed information.

Adaptive submodularity formalizes diminishing returns under partial observability and provides guarantees for adaptive greedy policies under its assumptions.

Potential ContextOS application:

```text
Select memory
        ↓
retrieve linked evidence
        ↓
observe evidence quality
        ↓
update candidate values
        ↓
select next item
```

This is a substantially richer research direction than static top-K selection.

---

# 16. Advanced Cache-Aware Optimization

The allocator should eventually optimize not just:

```text
which context?
```

but:

```text
which ordering?
```

Let:

$$
C=(c_1,c_2,\dots,c_n)
$$

and define:

$$
LCP(C_t,C_{t-1})
$$

as the stable-prefix length.

Then optimize:

$$
U'(C)
=
U(C)
+
\eta
LCP(C_t,C_{t-1})
$$

subject to:

$$
Tokens(C)\le B
$$

This turns prompt-cache stability into an optimization variable.

This matters because provider prompt caching is sensitive to repeated prefixes rather than merely to semantic similarity.

---

# 17. Advanced Robust Selection

ContextOS should eventually model uncertainty explicitly.

Instead of one score:

$$
s_i
$$

estimate:

$$
s_i\in[\ell_i,u_i]
$$

Then select using either:

```text
expected utility
```

or:

```text
worst-case utility
```

or:

```text
CVaR utility
```

For example:

$$
CVaR_\alpha(L)
$$

can penalize rare catastrophic stale-memory selections more heavily than ordinary expected-loss optimization.

This is relevant because one stale architectural decision can be much more damaging than several minor retrieval misses.

---

# 18. Learned Allocation — Future Research Track

Once enough traces exist, replace fixed weights.

Current system:

$$
Score(m)=
w^Tx(m)
$$

Future model:

$$
\hat{\Delta}_\theta(m\mid q,S,C)
$$

predicts the marginal value of adding candidate \(m\).

Features:

```text
lexical relevance
dense relevance
graph relevance
authority
freshness
evidence
memory age
model
task class
budget remaining
already selected context
cache state
repository state
historical usefulness
```

Then select:

$$
m^*
=
\arg\max_m
\frac{
\hat{\Delta}_\theta(m\mid q,S,C)
}{
Cost(m)
}
$$

---

# 19. Learned Policy Evaluation

Do not immediately train a policy and deploy it.

First use offline evaluation.

Treat selection as a contextual decision process.

Context:

$$
x=(q,S,B,M)
$$

Action:

$$
a=C
$$

Reward:

$$
r=
Success
-\lambda Cost
-\mu Latency
-\rho Risk
$$

Use logged policies and counterfactual estimators.

Doubly robust estimators are useful for off-policy evaluation because they combine a reward model with importance weighting; work also exists specifically for reducing variance and handling nonstationary settings.

This permits:

```text
evaluate new allocator
without deploying it blindly
```

---

# 20. Benchmark Matrix for Every PR

Every PR should produce a small regression report.

| Dimension        |  Current | New | Required      |
| ---------------- | -------: | --: | ------------- |
| Correctness      | baseline | new | no regression |
| Latency          | baseline | new | report        |
| Memory           | baseline | new | report        |
| Tokens           | baseline | new | report        |
| Cache reuse      | baseline | new | report        |
| Task success     | baseline | new | report        |
| Retrieval recall | baseline | new | report        |
| Failure cases    | baseline | new | report        |

Do not merge a performance optimization merely because average latency improves.

A good change may be:

```text
+2% latency
+12% task success
-20% tokens
```

while another may be:

```text
-30% latency
-15% task success
```

These require different decisions.

---

# 21. Required Regression Suite

Create one top-level command:

```bash
make test-research
```

which runs:

```text
unit tests
integration tests
store parity tests
index equivalence tests
retrieval benchmarks
allocator benchmarks
cache correctness tests
MCP conformance tests
```

And:

```bash
make benchmark-baseline
make benchmark-current
make benchmark-compare
```

The comparison should emit:

```text
PASS
WARN
FAIL
```

---

# 22. "Green" Criteria

For each PR define objective acceptance criteria.

## Green

```text
all correctness tests pass
no known invariant violated
no unexplained benchmark regression > threshold
research claims match implementation
reproducible benchmark
```

## Yellow

```text
correctness passes
performance tradeoff exists
additional evidence required
```

## Red

```text
new false cache reuse
incremental/full index mismatch
false freshness introduced
retrieval regression beyond threshold
protocol incompatibility
benchmark irreproducibility
unsupported theoretical claim
```

---

# 23. Recommended PR Order

The implementation order should remain:

```text
PR-01 Portable integrations
       ↓
PR-02 Worktree fingerprint
       ↓
PR-03 Graph relevance
       ↓
PR-04 Incremental indexing
       ↓
PR-05 True BM25
       ↓
PR-06 Semantic retrieval
       ↓
PR-07 Structured memory
       ↓
PR-08 Temporal validity
       ↓
PR-09 MCP correctness
       ↓
PR-10 Research integrity
       ↓
PR-11 Scientific benchmark
       ↓
PR-12 Longitudinal benchmark
```

Do not reorder these casually.

The dependency structure is intentional.

For example:

```text
PR-02 → cache correctness
PR-04 → scalable graph/index
PR-03 + PR-05 + PR-06 → retrieval
PR-07 + PR-08 → trustworthy memory
PR-10 → trustworthy claims
PR-11 → trustworthy experiments
PR-12 → research validation
```

---

# 24. Expected Architecture After PR-12

The final system should resemble:

```text
                    ┌────────────────────┐
                    │   Agent / Query    │
                    └─────────┬──────────┘
                              │
                    ┌─────────▼──────────┐
                    │   State Resolver   │
                    │ repo/git/session   │
                    └─────────┬──────────┘
                              │
           ┌──────────────────┼──────────────────┐
           │                  │                  │
           ▼                  ▼                  ▼
      Lexical IR         Dense IR           Code Graph
      BM25/BM25+        embeddings          PPR/diffusion
           │                  │                  │
           └──────────────────┼──────────────────┘
                              ▼
                    ┌──────────────────┐
                    │ Candidate Fusion │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Provenance/Time  │
                    │ Validity Filter  │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Context Selector │
                    │ utility / budget │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Cache Topology   │
                    │ stable prefix    │
                    └────────┬─────────┘
                             │
                    ┌────────▼─────────┐
                    │ Agent Context    │
                    └────────┬─────────┘
                             │
                             ▼
                       Task Outcome
                             │
                             ▼
                    ┌──────────────────┐
                    │ Evidence + Trace │
                    └────────┬─────────┘
                             │
                             ▼
                    ┌──────────────────┐
                    │ Memory Learning  │
                    └──────────────────┘
```

---

# 25. Final Research Thesis

The strongest version of ContextOS should not claim:

> "We built another RAG system."

It should instead investigate:

$$
\boxed{
\text{Persistent Engineering State}
+
\text{Temporal Provenance}
+
\text{Repository Graph}
+
\text{Budgeted Context Optimization}
+
\text{Cache-Aware Ordering}
+
\text{Longitudinal Learning}
}
$$

The central research question becomes:

$$
\boxed{
\text{How can an AI coding agent maintain the minimum sufficient persistent state required for successful future work, while minimizing token cost, latency, stale-context risk, and cache churn?}
}
$$

That question supports several rigorous subproblems:

$$
\text{retrieval}
\rightarrow
\text{selection}
\rightarrow
\text{validity}
\rightarrow
\text{ordering}
\rightarrow
\text{learning}
$$

and each stage can have its own theorem, algorithm, benchmark, and ablation study.

The most promising mathematical directions are therefore not isolated "fancy algorithms", but the interaction between:

$$
\boxed{
\text{Submodular / weak-submodular optimization}
}
$$

$$
\boxed{
\text{Personalized graph diffusion}
}
$$

$$
\boxed{
\text{Versioned provenance and temporal validity}
}
$$

$$
\boxed{
\text{Robust optimization under stale-context uncertainty}
}
$$

$$
\boxed{
\text{Adaptive sequential selection}
}
$$

$$
\boxed{
\text{Offline contextual-bandit policy learning}
}
$$

$$
\boxed{
\text{Statistically rigorous longitudinal evaluation}
}
$$

That combination is where the research value should concentrate—not in claiming novelty for BM25, embeddings, PageRank, MCP support, or submodular selection individually.

# 26. First Implementation Milestone

Before starting PR-01, create:

```text
docs/IMPLEMENTATION-RESEARCH-PLAN.md
docs/EXPERIMENT-PROTOCOL.md
benchmarks/
    schemas/
    fixtures/
    baselines/
    runners/
    reports/
```

and freeze the baseline:

```text
git rev-parse HEAD
ctxbench
go test ./...
```

Record all outputs.

That snapshot becomes the permanent comparison point for every subsequent PR.
