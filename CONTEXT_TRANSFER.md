# ContextOS — Transfer Context for New Chat

## Mission
Build a local-first, model-agnostic context infrastructure layer for software engineers who use multiple AI coding agents/models (Claude Code, Cursor, Codex, Gemini, etc.). The system should eliminate context loss across laptop restarts, new chats, agent switches, repositories, and long-running tasks while materially reducing token usage, latency, and model/API cost.

Primary UX goal: near-zero manual memory/context management.

## Original pain
Current workflow fragments engineering knowledge across:
- Claude Code sessions
- Cursor chats
- Codex sessions
- Gemini sessions
- ChatGPT conversations
- multiple repositories/branches
- ephemeral laptop/process state

Consequences:
- repeated rediscovery of architecture/decisions/failures
- context lost after restart/new chat
- manual copying of summaries between agents
- huge repeated input-token spend
- provider-specific prompt/cache inefficiency
- stale historical context can conflict with current code/instructions

## Core thesis
Do NOT build merely another “AI memory” or generic RAG system.

Core research/product thesis:

> Build a local-first Context OS for AI-native software engineering that adaptively chooses the minimum-sufficient, temporally-valid, evidence-backed context and model under explicit quality, latency, token, and cost constraints.

Working name: ContextOS.

Research algorithm name: ASC — Adaptive Sufficient Context.

## Strongest research question
How can an AI software-engineering agent dynamically allocate a bounded context budget across heterogeneous, temporally evolving engineering knowledge while jointly minimizing inference cost and preserving task success?

Formal target:

pi* = argmax_pi E[ Success - lambda*Cost - mu*Latency - nu*Waste - rho*Staleness ]
subject to Tokens <= B and P(success | state, task, action) >= tau.

## Important conclusion from prior-art review
Persistent coding-agent memory alone is too crowded to be novel.
Existing/native or emerging systems already cover parts of:
- Claude persistent memory / CLAUDE.md
- Cursor rules / AGENTS.md
- Codex AGENTS.md and durable-memory discussions
- Gemini project context
- Aider repo maps
- Sourcegraph/Cody repository context
- Atlassian Code Context
- MemGPT / A-MEM / MemoryOS / Mem0 / Zep / SimpleMem
- semantic caching
- adaptive coding-agent compression
- submodular/budgeted context selection

Therefore DO NOT claim novelty for any single component.

The target novelty is the JOINT controller / policy that combines:
1. retrieval
2. representation choice
3. compression
4. repository locality
5. version/authority-aware memory
6. cache reuse
7. prefetching
8. model routing
9. minimum-sufficient-context allocation
under one economic objective.

## Research areas classified green
Strong / potentially differentiated:
- minimum sufficient context
- joint retrieval + compression + cache optimization
- version/authority-aware engineering memory
- context prefetch based on developer working set
- cache-aware context ordering
- joint model + context routing
- end-to-end cost/quality controller

Most important flagship research problem:

Minimum Sufficient Context:

C* = argmin_C Tokens(C)
subject to P(success | C, q, M) >= tau.

Important distinction:
minimum tokens != minimum cost
because stable prefixes may be provider-cacheable and therefore more economical.

## System abstractions
Primary persisted object should NOT be a chat session.
Primary object:

WorkItem
- task/objective
- repository
- branch
- revision
- sessions
- agents used
- files/symbols touched
- decisions
- failures
- experiments
- open questions
- commits

This makes Claude/Cursor/Codex/Gemini interchangeable workers over shared engineering state.

## Memory types
Use typed memory, not generic text blobs:

1. Fact
2. Decision
3. Constraint
4. Failure / negative knowledge
5. Working state
6. Provenance / evidence

Negative knowledge is first-class:
Failure = attempt + outcome + cause + evidence + retryConditions.

## Evidence-backed claims
Represent durable knowledge as claims with provenance:

Claim = content + scope + revision validity + authority + evidence + confidence.

Example authority ordering (initial policy):
UserExplicit > CurrentCode > PassingTest > CurrentDoc > HistoricalDecision > ModelInference.

Memory states conceptually:
OBSERVED -> INFERRED -> VALIDATED -> CANONICAL -> STALE -> ARCHIVED.

## Temporal consistency
Every engineering claim needs repository/revision validity.

valid(memory, repo, revision)

Git changes should trigger impact analysis and invalidate/revalidate affected memories.

The system must explicitly avoid stale-memory poisoning.

## Repository graph
Graph G=(V,E) where nodes can include:
- repository
- branch
- commit
- file
- symbol
- class
- function
- test
- issue
- PR
- decision
- failure
- memory
- session

Edges can include:
- imports
- calls
- defines
- tests
- changed-by
- derived-from
- explains
- invalidates
- depends-on

For a task q, first localize:
G_q = Neighborhood(G, anchors(q))

Then perform hybrid retrieval inside G_q instead of globally.

## Context retrieval pipeline
Global knowledge
-> repo/branch/revision scope
-> temporal validity
-> graph localization
-> hybrid retrieval
-> trust/authority filtering
-> marginal utility estimation
-> budgeted selection
-> selective compression
-> cache-aware ordering
-> model routing
-> execution
-> outcome telemetry
-> memory + policy update

## Retrieve-or-not decision
Retrieval is itself optional.

EV(retrieve) = P(help|q,S)*Gain - retrievalCost - noiseRisk.

Retrieve only when EV > 0.
Optimal context can be empty.

## Representation choices
Candidate representations should include:
- raw code
- summary
- fact
- decision
- failure
- code slice
- symbol
- graph neighborhood
- test evidence

Select both WHAT and REPRESENTATION and AMOUNT.

## Context utility model
Candidate score should combine:
- semantic relevance
- structural relevance
- temporal freshness
- authority
- confidence
- task affinity
- expected reuse
- model-specific utility
- token cost
- staleness risk
- cache reuse

A baseline score:
Score_i = w1*Semantic + w2*Structural + w3*Temporal + w4*Authority + w5*Reuse + w6*TaskAffinity - w7*Tokens - w8*Staleness.

## Marginal utility
For context item m_i:
Delta_i = P(success|C) - P(success|C \ {m_i}).

Density_i = Delta_i / Tokens_i.

First implementation can use deterministic approximations.
Later learn a model for Delta_i from ablation traces.

Training trace format idea:
(task, candidate_context, selected_context, item, marginal_utility, tokens, outcome)

## Optimization theory
Simple context selection is a knapsack problem and NP-hard.
Submodular formulations are possible, but submodular context selection itself is not novel enough anymore.

Use established approximation machinery as a baseline, while focusing novelty on the engineering-specific utility model and joint controller.

Potential empirical test for approximate submodularity:

epsilon_sub = max_{A subset B, i} [Delta_i(B) - Delta_i(A)].

If near zero empirically, greedy allocation gets theoretical support.

Real utility can be non-monotone due to stale/conflicting context, so bad context should be hard-filtered before optimization.

## Cache hierarchy
Four levels:

L0 artifact cache:
- AST
- symbol graph
- repo map
- embeddings
- parsed docs

L1 semantic memory cache:
- reusable decisions
- failures
- architecture facts

L2 context-bundle cache:
- task -> selected context

L3 provider-prefix cache:
- stable token prefix

Important result:
minimum token count may not equal minimum dollar cost.
Stable prefix reuse can be economically superior.

Effective cost should include:
- uncached input
- cached input
- output
- retrieval
- latency or tool overhead as needed

## Cache keys
Artifact cache:
Hash(repoRevision, file/toolchain)

Context cache:
Hash(taskSemantics, repoRevision, policy, modelClass)

Provider cache:
provider-specific stable prefix identity / token sequence.

Avoid naive embedding-only response cache because same semantic query can mean different things across repos/environments.

## Cache value
Approximate:
V_cache = P(reuse) * avoidedCost - storageCost - stalenessRisk.

Potential research direction:
semantic cache management as online learning/contextual bandit.

## Prefetch
For memory/object m:
EV_prefetch(m) = P(next-needed|state) * avoidedLatency - prefetchCost.

Prefetch if EV > 0.

Learn developer working set W_t from:
- current repo
- branch
- open files
- recent edits
- current symbols
- task type
- recent retrievals
- historical reuse
- temporal locality

## Model routing
Choose model and context jointly:
(C*, M*) = argmax_{C,M} Quality(C,M) - lambda*Cost(C,M) - mu*Latency(C,M).

A cheap model with huge context may be more expensive than a premium model with compact, cacheable context.

## Prompt/context topology
Order stable content before volatile content when compatible with provider caching:

STABLE PREFIX
- repo identity
- stable architecture
- long-lived instructions
- tool definitions

VARIABLE SUFFIX
- current task
- fresh observations
- current file snippets
- dynamic state

Ordering is itself an optimization variable:
T* = argmax_T Quality(T(C)) + eta*CacheReuse(T(C)).

## POMDP perspective
True state is partially observable.
Belief state:
b_t(S)=P(S_t=S | observations).

Controller can be modeled as policy pi(a | belief, task).

Actions:
- retrieve
- compress
- prefetch
- cache/evict
- route model
- select context

Start heuristically, then learn from traces, then consider contextual bandits/RL.
Do NOT start with RL.

## Evaluation philosophy
Do not compare only on answer quality.
Measure the quality/cost frontier.

Core metrics:
- Pass@1 / task success
- input tokens
- output tokens
- effective dollar cost
- TTFT and end-to-end latency
- Recall@k / Precision@k
- Context Waste = 1 - usefulTokens / injectedTokens
- Minimum sufficient budget B_tau
- Rediscovery Rate
- Stale Context Rate
- Handoff Success
- Cache Hit Rate
- Cached Token Ratio
- Prefetch Hit Rate
- Prefetch Waste

Key headline metrics:
Task Success per 1K Input Tokens
Task Success per $.

Define:
B_tau = min B such that P(success) >= tau.

Define context elasticity:
E(B) = d Success / dB approximately [Success(B+delta)-Success(B)]/delta.

## Benchmark design
Need longitudinal software engineering tasks, not only single-shot SWE-bench.

Create traces like:
Session 1: Claude discovers architectural constraint
Session 2: machine/process restart
Session 3: Cursor continues
Session 4: Codex implements
Session 5: Gemini reviews
Repository changes between sessions.

Baselines:
B0 fresh agent
B1 full historical transcript
B2 naive vector RAG
B3 repo graph retrieval
B4 persistent memory
B5 adaptive compression
B6 budgeted context selection
B7 ContextOS heuristic
B8 ContextOS learned policy (later)

Run under budgets:
512, 1K, 2K, 4K, 8K, 16K, 32K tokens.

Adversarial cases:
- contradictory old/new decision
- same semantic query in different repos
- architecture changed over time
- cross-agent handoff
- laptop/process restart
- long trajectory
- negative knowledge
- no-context-needed case

## Research hypotheses
H1: For fixed task success target tau, B_tau(ContextOS) < B_tau(best baseline).
H2: ContextOS reduces effective monetary cost at equal task success.
H3: ContextOS reduces rediscovery rate.
H4: ContextOS improves cross-agent handoff success.
H5: ContextOS reduces stale-memory-induced errors.
H6: cache-aware context topology can beat pure token minimization economically.
H7: repository-aware localization improves context utility density.

## Existing prototype status
A runnable Go-based v0/v0.2 prototype was already created in /mnt/data/contextos.
Artifacts created previously:
- /mnt/data/contextos/ASC-1-SPEC.md
- /mnt/data/contextos/README.md
- /mnt/data/contextos/README-IMPLEMENTATION.md
- /mnt/data/contextos/ContextOS-v0.2.tar.gz
- /mnt/data/contextos/bin/contextd
- /mnt/data/contextos/bin/ctx

The prototype includes:
- SQLite persistence
- Git repository/revision detection
- lightweight source/symbol indexing
- durable facts/decisions/constraints/failures
- provenance/evidence
- stale/invalid filtering
- token-budgeted ASC allocation
- cache-aware context plans
- cross-process persistence
- MCP JSON-RPC server
- ctx CLI
- tests

The prototype uses a deterministic heuristic allocator, not the learned policy yet.

## Prototype commands
Examples:

ctx init
ctx work -title "Implement failover"
ctx session -agent codex
ctx event -session <id> -event observation -payload '...'
ctx remember -kind decision -authority user -content '...'
ctx plan -task "..." -model gpt-5.3-codex -budget 4000
ctx handoff -task "..." -target gemini
ctx resume
ctx stats

MCP mode:
contextd -repo /path/to/repo -mcp

## Next implementation milestone: v0.3 / real-agent integration
Proceed directly to implementation.

Priority order:
1. automatic session/transcript/event capture
2. proper repository graph
3. hybrid lexical + embedding retrieval
4. WorkItem lifecycle completion
5. decision/failure extraction
6. Git-driven memory invalidation
7. real context assembly
8. Claude Code adapter
9. Codex adapter
10. Cursor adapter
11. Gemini adapter
12. provider-specific cache telemetry
13. trace/observability UI
14. collect real traces for learned marginal-utility model

## Implementation principles
- local-first
- privacy-first
- cloud sync optional
- deterministic systems for facts mechanically derivable from Git/source/tests
- LLMs only for semantically difficult operations
- no Kubernetes/cloud complexity for v0.x
- SQLite + FTS + local/vector index + Tree-sitter + Git + MCP
- user should almost never manage memory manually

## Desired signature UX

ctx resume

should recover:
- where developer stopped
- current branch/task
- current implementation state
- decisions
- failures
- open questions
- likely next action

Cross-agent:

ctx handoff codex
ctx handoff claude
ctx handoff gemini

should hand over WorkItem state, not a manually generated chat summary.

## Provider integration goal
MCP is the primary neutral boundary.
The durable knowledge layer is provider-neutral.
Provider/model adapters are thin translation layers.

## Potential paper
Working title:
“Minimum-Sufficient Context Allocation for Long-Horizon Software Engineering Agents”

Potential contributions:
1. unified context-economic objective
2. version/authority/evidence-aware engineering memory model
3. adaptive context controller across retrieval, compression, cache, prefetch, routing
4. longitudinal cross-agent benchmark
5. evidence for minimum-sufficient-context frontier

## Potential patent angle
Do not patent “persistent AI memory.”
Potential claim direction:
A method/system that constructs a task-conditioned minimum-sufficient context by jointly evaluating semantic relevance, repository-graph locality, temporal validity, provenance/authority, expected marginal contribution, cache reuse, and model-specific cost, and dynamically updates the policy from observed task outcomes.

Potential dependent areas:
- Git/revision-aware invalidation
- negative knowledge
- cache-aware prompt ordering
- developer working-set prefetch
- cross-agent WorkItem continuity

Do not claim novelty/patentability without a formal jurisdiction-specific prior-art search.

## Current strategic positioning
Preferred wording:
“A local engineering context runtime that makes AI-assisted work persistent and cost-efficient.”

Avoid positioning as:
- another AI memory app
- generic RAG
- another chat archive
- universal agent connector

## Immediate next task in new chat
Start implementation of v0.3 directly.

First build slice:
1. inspect existing ContextOS-v0.2 source
2. preserve passing behavior
3. add automatic event capture abstraction
4. add real repo graph schema/indexer using Git + Tree-sitter
5. add hybrid retrieval interfaces with a local embedding implementation when available, with lexical fallback
6. add WorkItem/session lifecycle APIs
7. implement Git change impact + memory invalidation
8. expose MCP tools/resources
9. write integration tests
10. package runnable binaries

After that, wire the first real agent adapter, preferably Claude Code or Codex, while retaining MCP neutrality.

## Important: do not pause at planning
The previous instruction is to keep moving until the implementation is genuinely usable. Do not ask for confirmation between engineering steps. Make best-effort decisions and keep the prototype executable at each checkpoint.
