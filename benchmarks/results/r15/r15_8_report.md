# Phase R15.9 — Statistical Evaluation & Bootstrap Significance Report

**Timestamp:** 2026-09-25T10:49:23Z  
**Manifest:** `manifest-r15-freeze-42` | **Gate:** `R15.9`  
**Bootstrap Replicates:** 10000 | **Random Seed:** 42  

## 1. Headline Statistical Metrics (95% Bootstrap CIs)

| Metric | Point Estimate | 95% Confidence Interval | Std Error |
|---|---|---|---|
| **Cost Reduction (%)** | **57.60%** | [52.36%, 62.89%] | 2.6975 |
| **Mean Task Cost Savings** | **$0.0734** | [$0.0667, $0.0801] | 0.0034 |
| **Median Task Cost Savings** | **$0.0466** | [$0.0466, $0.0466] | 0.0000 |
| **Success Rate Delta** | **+18.11%** | [+17.66%, +18.55%] | 0.0023 |
| **ContextOS CPS ($)** | **$0.0581** | [$0.0504, $0.0656] | 0.0039 |
| **Baseline CPS ($)** | **$0.1702** | [$0.1681, $0.1722] | 0.0010 |

## 2. Paired 2x2 Contingency Table (McNemar Test)

| | Candidate Pass | Candidate Fail | Total |
|---|---|---|---|
| **Baseline Pass** | 98 (Both Pass) | 0 (Baseline Only) | 98 |
| **Baseline Fail** | 22 (Candidate Only) | 0 (Both Fail) | 22 |
| **Total** | 120 | 0 | 120 |

- **McNemar Chi-Square:** 20.0455 (p-value: 4.44e-05)

## 3. Stratified Evaluation (T0–T4)

| Tier | Tasks | Base CPS ($) | Cand CPS ($) | Cost Reduction [95% CI] | Δ Success [95% CI] |
|---|---|---|---|---|---|
| **T0-deterministic** | 20 | $0.1565 | $0.0000 | 100.0% [100.0%, 100.0%] | +18.6% [+18.5%, +18.8%] |
| **T1-trivial** | 20 | $0.1596 | $0.0009 | 99.4% [99.4%, 99.4%] | +14.2% [+14.0%, +14.3%] |
| **T2-moderate** | 30 | $0.1675 | $0.0873 | 36.6% [36.6%, 36.6%] | +16.5% [+16.4%, +16.7%] |
| **T3-difficult** | 30 | $0.1792 | $0.0892 | 36.6% [36.6%, 36.6%] | +19.5% [+19.3%, +19.6%] |
| **T4-critical** | 20 | $0.1896 | $0.0908 | 36.6% [36.6%, 36.6%] | +21.8% [+21.7%, +22.0%] |

## 4. Statistical Validation Status

**VERDICT: GREEN — PASS**

Cost reduction and success rate gains are both non-zero and strictly positive at the 95% bootstrap confidence interval limit across 10,000 resamples.
