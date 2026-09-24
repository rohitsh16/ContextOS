# ContextOS Dashboard — Metrics, Telemetry & Deployment Guide

This guide covers the **ContextOS Web Dashboard** (`ctx ui`), detailing its two-tier efficiency architecture, mathematical metrics glossary, and production deployment models (live daemon or static GitHub Pages).

---

## 1. The Two-Tier Efficiency Model

When using AI coding agents, cost and latency come from two distinct layers:
1. **Context Pruning (Volume)**: Minimizing the token footprint sent to the model.
2. **Provider Prompt Caching (Asymmetry)**: Structuring tokens to maximize KV cache hits.

ContextOS optimizes both layers simultaneously:

```text
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

## 2. Metric Glossary & Mathematical Formulations

### 2.1 Uncompressed Baseline
- **Definition**: The token volume and billable cost incurred without ContextOS intervention (e.g. unpruned file dumps, conversation history).
- **Calculation**:
  $$\text{Baseline Tokens} = \text{Budget Ceiling} \quad \text{or} \quad \max(1500, \text{Memories} \times 650)$$

### 2.2 Injected / Pruned Tokens
- **Injected Tokens**: Tokens selected for the prompt package ($\text{Tokens}(C^*)$).
- **Pruned Tokens**: $\text{Baseline Tokens} - \text{Injected Tokens}$.
- **Token Reduction**:
  $$\text{Reduction \%} = \frac{\text{Baseline Tokens} - \text{Injected Tokens}}{\text{Baseline Tokens}} \times 100$$

### 2.3 Stable Prefix vs. Variable Context
- **Stable Prefix ($C_{\text{stable}}$)**: Immutable architectural invariants, constraints, and decisions placed at the beginning of the prompt to ensure identical prefix token sequences for KV caching.
- **Variable Context ($C_{\text{var}}$)**: Task-specific candidates, recent edits, and ephemeral traces placed after the prefix.

### 2.4 KV-Cache Savings (Level 2)
For models supporting prompt caching (Anthropic Claude 3.5/3.7, OpenAI GPT-4o, Google Gemini 2.5):
$$\text{Cost}_{\text{turn}} = C_{\text{stable}} \cdot P_{\text{cached}} + C_{\text{var}} \cdot P_{\text{input}} + \text{OutputTokens} \cdot P_{\text{output}}$$

Where $P_{\text{cached}}$ is typically 75%–90% lower than $P_{\text{input}}$.

---

## 3. Dashboard Deployment Architecture

The ContextOS dashboard (`internal/ui/assets/`) is engineered with **Vanilla HTML, CSS, and Modern JavaScript** (`index.html`, `style.css`, `app.js`) without external runtime dependencies.

### Deployment Modes:

1. **Local Live Daemon Mode (`ctx ui`)**:
   Runs a local HTTP server connected directly to SQLite WAL (`.contextos/context.db`), providing real-time trace inspection and interactive memory controls.

2. **Static Serverless Hosting (GitHub Pages / CDN)**:
   Pre-exports JSON state snapshots to a static web directory without requiring a Go backend:

```text
dist-dashboard/
├── index.html                  # Dashboard frontend structure
├── style.css                   # Theme and layout styles
├── app.js                      # Dual-mode dashboard client
└── data/                       # Static FileStore export
    ├── status.json             # System info, total memories, commit hash
    ├── memories.json           # Active Memory Hub items
    ├── sessions.json           # Agent sessions & telemetry traces
    └── report.json             # Benchmark savings report
```

### Static Build Command:
```bash
ctx export-dashboard --output dist-dashboard/
```
Deploy the resulting directory to `gh-pages` or any static CDN.
