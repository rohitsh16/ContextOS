# ContextOS Dashboard Metrics & Efficiency Guide

This guide explains the efficiency, cost, and retrieval metrics displayed in the ContextOS Web Dashboard (`ctx ui`) and benchmark reports. It provides both **plain-English (layman) explanations** with real-world analogies and the **exact mathematical formulas** implemented in the runtime.

---

## The Two-Tier Efficiency Model

When using AI coding agents, cost and latency come from two distinct layers:
1. **How much context you send** (Token volume)
2. **How the cloud AI provider bills for that context** (Provider KV prompt-caching)

ContextOS optimizes both layers simultaneously:

```
                                  Naïve Agent Workflow
                    ┌───────────────────────────────────────────────┐
                    │ Full Repository Dumps / Unpruned History      │
                    │ Baseline: 1,500 – 4,000+ Tokens               │
                    └───────────────────────┬───────────────────────┘
                                            │ Full Price ($3.00/M)
                                            ▼
                                   Expensive LLM Bill
                                 ($0.0045 – $0.012 / call)

─────────────────────────────────────────────────────────────────────────────

                              ContextOS Optimized Workflow
  Level 1: Direct Context Pruning (Empirical)
  ┌───────────────────────────────────────────────────────────────┐
  │ 8-Pass ASC-1 Engine + Block-Max WAND Pruning                  │
  │ • Drops irrelevant files, stale symbols, redundant memories   │
  │ • Cuts context to Minimum Sufficient Decision Units (41 tok)  │
  └───────────────────────────────┬───────────────────────────────┘
                                  │ 73.4% – 97.3% Fewer Tokens
                                  ▼
  Level 2: Cloud KV Prompt-Caching (Provider Upside)
  ┌───────────────────────────────────────────────────────────────┐
  │ Deterministic Stable Prefix Partitioning                      │
  │ • Fixed architectural facts & invariants placed at head       │
  │ • Triggers Anthropic / OpenAI / Gemini KV cache (90% off)     │
  └───────────────────────────────┬───────────────────────────────┘
                                  │ Compounded 90% Discount on Hits
                                  ▼
                         Ultra-Low Final Bill
                       ($0.00007 / call — 98.6% Total Savings)
```

---

## Metric Glossary & Detailed Breakdown

### 1. Uncompressed Baseline

* **Layman Explanation:**
  What you would have paid if you weren't using ContextOS. When standard AI assistants answer questions, they dump entire files, broad grep results, or sprawling multi-turn conversation logs into the prompt. The AI reads everything from scratch, and you get billed full price for every single token.
* **The Analogy:**
  You need to ask a specialist lawyer a quick question. Instead of giving them a 1-page summary, you drop a **100-page cardboard box** of unsorted documents on their desk. They charge you their maximum hourly rate to read through the entire box.
* **How It Is Calculated:**
  $$\text{Baseline Tokens} = \text{Trace Budget (e.g. 2,500 tokens)} \quad \text{or} \quad \max(1500, \text{Memories} \times 650)$$
  $$\text{Baseline Cost} = \frac{\text{Baseline Tokens}}{1{,}000{,}000} \times \text{Model Input Rate}$$

---

### 2. Direct Cost Saved (Level 1: Direct Token Pruning)

* **Layman Explanation:**
  The money you save **instantly and guaranteed** simply by sending dramatically fewer words to the AI. ContextOS strips out boilerplate, irrelevant dependencies, and outdated comments, packing only the exact 40 to 400 tokens necessary for the model to make an accurate engineering decision.
* **The Analogy:**
  Your executive assistant reviews the 100-page box before the lawyer sees it, pulls out the exact **2 pages** that matter, and shreds the rest. You immediately avoid paying reading fees on 98 pages.
* **How It Is Calculated:**
  $$\text{Net Tokens Saved} = \max(0, \text{Budget Requested} - \text{Tokens Actually Selected})$$
  $$\text{Direct Cost Saved} = \frac{\text{Net Tokens Saved}}{1{,}000{,}000} \times \text{Model Input Rate}$$
  $$\text{Savings Percentage} = \frac{\text{Net Tokens Saved}}{\text{Budget Requested}} \times 100\%$$

---

### 3. KV Cache & Stable Prefix Hits (Level 2: Provider Prompt Caching)

* **Layman Explanation:**
  The AI provider's high-speed **short-term memory**. When modern LLMs (Anthropic Claude 3.7 Sonnet, OpenAI GPT-4o, Google Gemini 2.5 Pro) process text, they perform massive mathematical matrix calculations on each word and store the resulting attention states in GPU memory (the **Key-Value / KV Cache**). If your next prompt begins with the exact same prefix as before, the cloud provider doesn't recalculate it—they just read it out of GPU RAM and give you an **80% to 90% discount** on those words.
* **How ContextOS Maximizes This:**
  ContextOS partitions context into two sections:
  1. **Stable Prefix:** Invariable architectural decisions, constraints, and conventions, deterministically sorted at the very head of the prompt.
  2. **Variable Context:** Ephemeral task-specific variables placed at the tail.
  Because the Stable Prefix remains identical across multi-turn prompts, cloud LLM providers grant prompt-cache hits on almost every call.
* **The Analogy:**
  You call the lawyer back 5 minutes later with a follow-up question. Because page 1 was the exact same background facts you discussed earlier, the lawyer remembers it from their desk notes and only charges pennies to review it again.

---

### 4. Cost w/ Cloud Cache

* **Layman Explanation:**
  The **actual, bottom-line bill** you pay to your AI cloud provider. This reflects both cost-saving mechanisms combined: you sent far fewer words (pruning), and for the words that were already cached in the cloud GPU, you paid the 90%-off rate.
* **How It Is Calculated:**
  $$\text{Active Rate} = \begin{cases} \text{Model Cached Input Rate} & \text{if KV Cache Hit (e.g. \$0.30 / M)} \\ \text{Model Standard Input Rate} & \text{if KV Cache Miss (e.g. \$3.00 / M)} \end{cases}$$
  $$\text{Actual Cost} = \frac{\text{Selected Tokens}}{1{,}000{,}000} \times \text{Active Rate}$$

---

### 5. Compound Savings

* **Layman Explanation:**
  The **multiplication effect** of combining Level 1 pruning with Level 2 prompt-caching.
  * *First*, you cut token volume by **70% to 97%**.
  * *Second*, on the small fraction you do send, you receive another **90% discount** whenever the prefix hits the cache.
  * When you multiply both discounts, your net API bill drops by **97% to 99%**. That total combined reduction is your **Compound Savings**.
* **How It Is Calculated:**
  $$\text{Compound Cost Saved} = \text{Baseline Cost} - \text{Actual Cost w/ Cloud Cache}$$
  $$\text{Compound Savings Percentage} = \frac{\text{Compound Cost Saved}}{\text{Baseline Cost}} \times 100\%$$

---

### 6. Retrieval Touch Ratio & Search Space Pruned

* **Layman Explanation:**
  How much of your codebase ContextOS had to read to find the right context. In traditional search, a system performs an exhaustive scan (Touch Ratio = 100%). With **Block-Max WAND dynamic score pruning**, ContextOS skips entire blocks of irrelevant postings, inspecting only **0.6% to 3.0%** of repository entities.
* **The Analogy:**
  Instead of reading every book in a library from cover to cover to find a quote, you use a curated index to jump straight to the exact shelf and page, ignoring 98% of the building.
* **Key Numbers:**
  * **Touch Ratio:** `2.1%` (Empirical average across large codebases)
  * **Search Space Pruned:** `97.9%` eliminated
  * **Latency Speedup:** `2.06x` faster retrieval (0.91ms P50 vs 1.88ms exhaustive)

---

### 7. SLA Status & Adaptive Context Throttling

* **Layman Explanation:**
  A safety mechanism for massive monorepos. If a complex graph search or large repository scan consumes more than 50% of your configured query deadline (e.g. 250ms of a 500ms timeout SLA), ContextOS automatically down-throttles the token budget to `min_budget` (e.g. 400 tokens) and returns minimum-sufficient context immediately.
* **Why It Matters:**
  Guarantees that your IDE (Cursor, Claude Code, Antigravity) never experiences a tool timeout or freeze, maintaining sub-second interactive response times.

---

## Quick Reference Summary Table

| Term | Category | Layman Definition | Empirical Benchmark Value |
| :--- | :--- | :--- | :--- |
| **Uncompressed Baseline** | Token / Cost | Cost of raw, unpruned files or full transcript | 1,500 – 2,500 tokens ($0.0026–$0.0044) |
| **Direct Tokens Pruned** | Tokens | Words eliminated before prompt submission | 1,459 tokens saved (41 tok selected) |
| **Direct Cost Saved** | Cost | Money saved by sending fewer words at standard rate | $0.0025 saved per query (97.3% cut) |
| **KV Cache Hit Rate** | Provider Cache | Frequency of matching the deterministic Stable Prefix | 85% – 100% across multi-turn sessions |
| **Cost w/ Cloud Cache** | Cost | Final bill after pruning + provider cache discounts | $0.00007 per query |
| **Compound Savings** | Cost | Combined multi-tier discount percentage | **97.3% – 98.6%** total cost reduction |
| **Touch Ratio** | Retrieval | Percentage of codebase inspected during search | **0.6% – 3.0%** (97.0%–99.4% pruned) |
| **Speedup (P50)** | Retrieval | Retrieval acceleration vs exhaustive scan | **2.06x** (0.91ms vs 1.88ms) |
