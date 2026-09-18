# ContextOS Empirical Evaluation & Benchmark Report

**Repository:** `ContextOS` (`main` @ `5788e861a2`)  
**Generated At:** `2026-09-18 17:10:08 UTC`  
**Evaluation Framework:** ContextOS ASC-1 6-Pass Context Runtime  

### Badges
[![ContextOS Token Savings](https://img.shields.io/badge/Token_Pruned-50.5%25-brightgreen.svg)](#)
[![KV Cache Hit Rate](https://img.shields.io/badge/KV--Cache_Hits-31.3%25-blue.svg)](#)
[![Raw Context Reduction](https://img.shields.io/badge/Raw_Reduction-96.2%25-orange.svg)](#)

---

## 1. Executive Summary

| Metric | Measured Result | Significance / Baseline |
| :--- | :--- | :--- |
| **Total Real-World Allocations** | **16 traces** | Live agent turns and IDE invocations |
| **Total Requested Budget Ceiling** | **38,800 tokens** | Upper token constraints set by developers/tools |
| **Actual Context Selected & Injected** | **19,222 tokens** | Minimum-sufficient packed context |
| **Net Tokens Pruned / Saved** | **19,578 tokens (50.5%)** | **Immediate token reduction vs. budget ceiling** |
| **KV-Cache Hit Rate** | **5 / 16 (31.3%)** | **Byte-identical Stable Prefix reuse** (75–90% provider discount) |
| **Reduction vs. Raw Repository Dumps** | **96.2% reduction** | vs. ~32k raw uncompressed dumps per turn |
| **Estimated Cost Reduction** | **$0.059** | Based on standard $3.00 / 1M token input pricing |

---

## 2. Engineering Knowledge & Decision Inventory

ContextOS manages **8 total durable memories** across the codebase:

- **Architectural Decisions (`decision`):** 4 (durable architectural choices tagged to git revisions)
- **Negative Knowledge (`failure`):** 3 (documented dead ends preventing repeating errors)
- **Engineering Constraints (`constraint`):** 1 (KV cache ordering and budget limits)
- **General Facts (`fact`):** 0

---

## 3. Allocation Trace History

| # | Task / Objective | Model | Budget | Injected | Saved | KV-Cache | Timestamp |
| :---: | :--- | :---: | :---: | :---: | :---: | :---: | :---: |
| 1 | greedy allocation token budget | `gpt-5.3-codex` | 1500 | 681 | **+819 (54.6%)** | MISS | 2026-09-15T10:46:57Z |
| 2 | greedy allocation token budget | `claude` | 4000 | 681 | **+3319 (83.0%)** | MISS | 2026-09-15T10:47:31Z |
| 3 | Fix duplicate payment processing | `local` | 2000 | 1273 | **+727 (36.4%)** | MISS | 2026-09-15T18:04:47.592955Z |
| 4 | Implement distributed lock with Redis... | `claude-3-7-sonnet` | 2500 | 780 | **+1720 (68.8%)** | MISS | 2026-09-18T16:28:01.899505Z |
| 5 | Implement distributed lock with Redis... | `claude-3-7-sonnet` | 2500 | 780 | **+1720 (68.8%)** | **HIT** | 2026-09-18T16:29:31.094562Z |
| 6 | Implement distributed lock failover w... | `local` | 2500 | 863 | **+1637 (65.5%)** | MISS | 2026-09-18T16:38:07.53018Z |
| 7 | Implement distributed lock with Redis... | `local` | 2500 | 946 | **+1554 (62.2%)** | MISS | 2026-09-18T16:38:17.244963Z |
| 8 | Implement distributed lock with Redis... | `local` | 2500 | 946 | **+1554 (62.2%)** | **HIT** | 2026-09-18T16:38:28.220596Z |
| 9 | Implement Redis redlock distributed l... | `local` | 2500 | 275 | **+2225 (89.0%)** | MISS | 2026-09-18T16:38:45.749613Z |
| 10 | ContextOS Real-World UI Dashboard and... | `local` | 2500 | 1183 | **+1317 (52.7%)** | MISS | 2026-09-18T16:38:53.791358Z |
| 11 | ContextOS Real-World UI Dashboard and... | `local` | 2500 | 1183 | **+1317 (52.7%)** | **HIT** | 2026-09-18T16:39:02.01752Z |
| 12 | Implement distributed redlock timeout | `claude` | 2000 | 375 | **+1625 (81.3%)** | MISS | 2026-09-18T16:39:19.33692Z |
| 13 | Integrate ContextOS with Antigravity ... | `local` | 1800 | 1792 | **+8 (0.4%)** | MISS | 2026-09-18T16:51:32.43361Z |
| 14 | ContextOS Real-World UI Dashboard and... | `local` | 2500 | 2488 | **+12 (0.5%)** | MISS | 2026-09-18T16:51:36.549945Z |
| 15 | ContextOS Real-World UI Dashboard and... | `local` | 2500 | 2488 | **+12 (0.5%)** | **HIT** | 2026-09-18T16:51:39.669203Z |
| 16 | ContextOS Real-World UI Dashboard and... | `local` | 2500 | 2488 | **+12 (0.5%)** | **HIT** | 2026-09-18T16:51:45.528985Z |

---

## 4. Methodology & Optimization Architecture

ContextOS uses the **ASC-1 6-Pass Algorithmic Pipeline**:

1. **BM25 Lexical Retrieval**: Identifies relevant engineering symbols and memories matching lexical tokens.
2. **HashSemantic Cosine Similarity**: Matches concepts across varied wording without external model latency.
3. **Reciprocal Rank Fusion & Authority Scoring**: Combines lexical and semantic ranks with user authority multipliers.
4. **Knapsack Density Packing**: Selects highest utility per token until budget boundary.
5. **(1 - 1/e) Sviridenko Singleton Rescue**: Rescues high-value monolithic decisions from being starved by small items.
6. **KV-Cache Alignment Partition**: Enforces byte-identical ordering on the Stable Prefix for Claude/Gemini cache hits.

*Report generated autonomously by ContextOS.*  
