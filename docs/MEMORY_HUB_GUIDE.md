# Memory Hub Reference Guide: Constraints, Facts & Code AST

ContextOS's **Memory Hub** equips AI coding agents with durable engineering memory, preventing repetitive mistakes and eliminating context bloat. Rather than dumping raw files into LLM prompts, ContextOS categorizes knowledge into precise **typed memories** and ranks them using the **ASC-1 (Adaptive Semantic Compression)** allocator.

This guide provides definitions, best practices, and field-specific examples for **Constraints**, **Facts**, and **Code AST** across six major disciplines:
1. [Software Engineering](#1-software-engineering)
2. [Artificial Intelligence & Machine Learning](#2-artificial-intelligence--machine-learning)
3. [Bio Research & Bioinformatics](#3-bio-research--bioinformatics)
4. [Game Development](#4-game-development)
5. [Design & UI/UX](#5-design--uiux)
6. [General Research & Data Science](#6-general-research--data-science)

---

## The ContextOS Memory Taxonomy

ContextOS structures context into five core types:

| Memory Kind | Definition | Key Purpose in Prompt Optimization |
| :--- | :--- | :--- |
| **`constraint`** | Non-negotiable architectural rules, guardrails, compliance laws, or API invariants. | Top priority in ASC-1 packing. Prevents agents from breaking system invariants or introducing regressions. |
| **`fact`** | Objective environmental truths, configuration keys, live endpoints, schemas, or dependencies. | High semantic affinity. Informs the agent of live realities without re-querying the filesystem or network. |
| **`code` (AST)** | Structural symbol definitions (functions, classes, structs, interfaces) extracted via AST indexing. | Provides interface signatures and file coordinates (`path:line`), saving 80–90% of tokens vs. reading whole source files. |
| **`decision`** | Architectural rationale, ADR records, and design choices. | Guides the agent on *why* a particular pattern is favored over alternatives. |
| **`failure`** | Documented bug root-causes, pitfalls, and compilation/runtime edge cases. | Negative evidence weighting that stops agents from repeating past errors. |

---

## Anatomy of a Memory Record

Every entry stored in ContextOS conforms to the following schema:

```json
{
  "kind": "constraint",
  "content": "All database mutations must execute within a transactional boundary with rollback handlers.",
  "scope": "repo",
  "authority": "user",
  "confidence": 0.98,
  "location": "backend/db/transaction.go"
}
```

- **`kind`**: `constraint` | `fact` | `decision` | `failure` | `code`
- **`content`**: Crisp, high-information-density description (aim for 15–40 tokens).
- **`scope`**: `repo` (applies across this repository), `branch` (isolated to current feature branch), or `global` (system-wide).
- **`authority`**: `user` (1.0 weight), `commit` (0.9 weight), `doc` (0.85 weight), `test` (0.95 weight), `inference` (0.75 weight).
- **`confidence`**: Floating point between `0.1` and `1.0`. Higher confidence prevents automatic pruning.
- **`location`**: Associated source file coordinate or external URL.

---

## Domain Examples by Field

---

### 1. Software Engineering

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Migrations):**
  > `"Database schema changes must be strictly backwards-compatible: column drops require a 2-phase deployment cycle with deprecation flags."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (Concurrency):**
  > `"Shared state in the sync worker pool must never use naked mutexes; all locks must use sync.RWMutex with defer unlock pattern."`  
  > *Authority: commit | Confidence: 0.95 | Scope: repo*
- **Constraint 3 (Security/Auth):**
  > `"All API endpoints under /v2/admin/* must pass through the TenantIsolationMiddleware and require HMAC-SHA256 signature verification."`  
  > *Authority: doc | Confidence: 1.0 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Redis Configuration):**
  > `"Production Redis cluster operates on port 6379 with a 2,500ms read timeout and a maximum of 20 idle pool connections."`  
  > *Authority: doc | Confidence: 0.90 | Scope: repo*
- **Fact 2 (Binary Signing on macOS):**
  > `"Binaries compiled with CGO on Apple Silicon (arm64 Darwin) must be signed with codesign -s - -f to prevent AMFI SIGKILL termination."`  
  > *Authority: test | Confidence: 1.0 | Scope: repo*
- **Fact 3 (Event Store Limit):**
  > `"SQLite WAL mode requires PRAGMA busy_timeout=5000 and PRAGMA synchronous=NORMAL for concurrent multi-process writes."`  
  > *Authority: commit | Confidence: 0.95 | Scope: repo*

#### Code AST (Symbol Signatures)
- **Function:**  
  `function Connect func (m *MySQLDatabase) Connect() (*sql.DB, error) backend/db/mysql.go:20`
- **Struct Definition:**  
  `type PostResponse struct { ID int64, Title string, Content string, Published bool } backend/dto/dto.go:46`
- **Interface:**  
  `type StorageEngine interface { Get(key string) ([]byte, error); Put(key string, val []byte) error } internal/store/store.go:18`

---

### 2. Artificial Intelligence & Machine Learning

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Vector Embeddings):**
  > `"All dense embedding vectors must be normalized with L2 Euclidean unit norm prior to cosine dot product calculation."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (Gradient Accumulation):**
  > `"When training on batch size > 64, gradient clipping must be capped at max_norm=1.0 to prevent gradient explosion in cross-attention."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*
- **Constraint 3 (Deterministic Seeding):**
  > `"All PyTorch dataloader workers must set torch.manual_seed(42) and np.random.seed(42) in worker_init_fn for experiment reproducibility."`  
  > *Authority: commit | Confidence: 0.90 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Hardware Acceleration):**
  > `"FlashAttention-2 requires CUDA compute capability >= sm_80 (Ampere/Hopper) and torch.bfloat16 tensor precision."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*
- **Fact 2 (Model Context Window):**
  > `"Claude 3.7 Sonnet supports 200k token context with 128k output limit; prompt caching triggers on prefixes >= 1,024 tokens."`  
  > *Authority: doc | Confidence: 0.90 | Scope: global*
- **Fact 3 (Tokenizer Offsets):**
  > `"The tiktoken cl100k_base tokenizer encodes Unicode surrogate pairs into separate byte tokens, causing character offset shifts in spans."`  
  > *Authority: test | Confidence: 0.92 | Scope: repo*

#### Code AST (Symbol Signatures)
- **PyTorch Module:**  
  `class MultiHeadAttention(nn.Module): def forward(self, q, k, v, mask=None) -> Tensor: models/transformer.py:84`
- **Embedding Function:**  
  `def generate_dense_embeddings(texts: List[str], batch_size: int = 32) -> np.ndarray: pipeline/embed.py:112`
- **Loss Class:**  
  `class ContrastiveInfoNCELoss(nn.Module): def __init__(self, temperature: float = 0.07): loss/contrastive.py:28`

---

### 3. Bio Research & Bioinformatics

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Genome Coordinates):**
  > `"All genomic variant positions must strictly adhere to 1-based closed coordinates on GRCh38/hg38 reference assembly."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (VCF Quality Filtering):**
  > `"Variants with depth DP < 10 or mapping quality MQ < 40 must be filtered with low_qual tag before population frequency annotation."`  
  > *Authority: commit | Confidence: 0.95 | Scope: repo*
- **Constraint 3 (Non-Canonical Splicing):**
  > `"Never impute non-canonical splice junctions without supporting RNA-seq junction reads with minimum 5 split alignments."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Gene Coordinates):**
  > `"BRCA1 is located on chromosome 17 (NC_000017.11: 43044295-43125483, reverse strand) and encodes an 1,863 amino acid protein."`  
  > *Authority: doc | Confidence: 1.0 | Scope: repo*
- **Fact 2 (AlphaFold Confidence Thresholds):**
  > `"AlphaFold structures with pLDDT > 90 have high accuracy; pLDDT 70-90 indicate well-modeled backbone; pLDDT < 50 indicates intrinsic disorder."`  
  > *Authority: doc | Confidence: 0.95 | Scope: global*
- **Fact 3 (gnomAD Exome Version):**
  > `"Population allele frequency threshold for rare variant classification is AF < 0.001 in gnomAD v4.1 non-Finnish European exomes."`  
  > *Authority: test | Confidence: 0.95 | Scope: repo*

#### Code AST (Symbol Signatures)
- **VCF Parser Function:**  
  `def parse_vcf_record(line: str, ref_fasta: FastaFile) -> VariantRecord: bio/vcf_reader.py:45`
- **Structural Analyzer Class:**  
  `class AlphaFoldConfidenceAnalyzer: def get_domain_plddt(self, cif_path: str) -> Dict[str, float]: bio/af_metrics.py:88`
- **Splicing Evaluator:**  
  `def evaluate_splice_junction(chrom: str, start: int, end: int, strand: str) -> SpliceScore: bio/splicing.py:130`

---

### 4. Game Development

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Frame Rate Budget):**
  > `"Main rendering thread budget is 16.6ms (60 FPS); draw calls must not exceed 120 per frame on target mobile profiles."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (Zero Garbage Collection in Update):**
  > `"MonoBehaviour.Update() and FixedUpdate() loops must never allocate heap objects (no new, no LINQ, no string concatenation)."`  
  > *Authority: commit | Confidence: 0.98 | Scope: repo*
- **Constraint 3 (Physics Determinism):**
  > `"All gameplay physics calculations must execute in FixedUpdate() using fixedDeltaTime to maintain deterministic replay sync."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Render Pipeline):**
  > `"Unity Universal Render Pipeline (URP 14.0) is configured with Forward+ rendering path and screen-space ambient occlusion."`  
  > *Authority: doc | Confidence: 0.90 | Scope: repo*
- **Fact 2 (TTK Balance Profile):**
  > `"Combat balance baseline specifies 450ms Time-To-Kill (TTK) for close-range assault rifles at effective engagement range < 15m."`  
  > *Authority: test | Confidence: 0.90 | Scope: repo*
- **Fact 3 (NavMesh Layer Mask):**
  > `"NavMesh agent walkable layer is mask 0x01; water hazards are designated layer 0x04 with pathfinding cost multiplier of 8.0."`  
  > *Authority: commit | Confidence: 0.95 | Scope: repo*

#### Code AST (Symbol Signatures)
- **Procgen Algorithm Class:**  
  `class WaveFunctionCollapseGenerator : MonoBehaviour { public void GenerateGrid(Vector2Int size, BiomeConfig biome); } Procgen/WFC.cs:34`
- **Combat Balance Component:**  
  `public struct WeaponDamageModel { public float BaseDamage; public AnimationCurve FalloffCurve; } Combat/DamageModel.cs:15`
- **Player Controller Method:**  
  `public void ApplyKnockback(Vector3 direction, float force, ForceMode mode) Characters/PlayerMotor.cs:210`

---

### 5. Design & UI/UX

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Touch Target Accessibility):**
  > `"All interactive buttons and navigation links must meet minimum touch target sizing of 44x44 CSS pixels per WCAG 2.1 AAA."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (Color Contrast Ratio):**
  > `"Text-to-background contrast ratio must be >= 4.5:1 for standard body copy and >= 3.0:1 for large display headers (>= 18pt/24px)."`  
  > *Authority: doc | Confidence: 0.98 | Scope: repo*
- **Constraint 3 (Pure Vanilla CSS):**
  > `"Do not use Tailwind utility classes in component templates; use scoped Vanilla CSS design tokens from style.css."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Design Tokens Palette):**
  > `"Primary accent color is --accent-indigo: hsl(238, 82%, 67%); emerald success token is --accent-emerald: hsl(158, 64%, 52%)."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*
- **Fact 2 (Typography Hierarchy):**
  > `"Font stack specifies 'Outfit' (500/600/700) for headers, 'Inter' (400/500/600) for body UI, and 'JetBrains Mono' for metrics."`  
  > *Authority: doc | Confidence: 0.90 | Scope: repo*
- **Fact 3 (Transition Timing Standards):**
  > `"Standard interactive micro-animations use transition: all 0.15s cubic-bezier(0.4, 0, 0.2, 1) for button hover and tab activation."`  
  > *Authority: commit | Confidence: 0.90 | Scope: repo*

#### Code AST (Symbol Signatures)
- **Navigation Component:**  
  `function renderNavTabs(activeTab: string): HTMLElement frontend/components/NavTabs.ts:18`
- **Modal Dialog Factory:**  
  `class ModalDialog { public open(): void; public close(): void; public setContent(node: HTMLElement): void } frontend/ui/Modal.ts:42`
- **Color Token Utility:**  
  `export const getThemeToken = (tokenName: CSSCustomProperty): string => { ... } frontend/tokens/theme.ts:12`

---

### 6. General Research & Data Science

#### Constraints (Guardrails & Rules)
- **Constraint 1 (Statistical Reporting):**
  > `"All statistical hypothesis tests must report sample size N, 95% confidence intervals, and effect size Cohen's d alongside p-values."`  
  > *Authority: user | Confidence: 1.0 | Scope: repo*
- **Constraint 2 (Data Leakage Prevention):**
  > `"Feature normalization scalers (MinMaxScaler / StandardScaler) must be fit exclusively on training splits before transforming test splits."`  
  > *Authority: commit | Confidence: 0.98 | Scope: repo*
- **Constraint 3 (Anonymization):**
  > `"Subject IDs and timestamps must be pseudonymized using HMAC-SHA256 with an ephemeral research salt prior to CSV serialization."`  
  > *Authority: doc | Confidence: 1.0 | Scope: repo*

#### Facts (Environment & Configuration)
- **Fact 1 (Dataset Specifications):**
  > `"The benchmark evaluation dataset comprises 14,250 verified clinical trial publications from ClinicalTrials.gov API v2 (2020–2025)."`  
  > *Authority: doc | Confidence: 0.95 | Scope: repo*
- **Fact 2 (Metric Definition):**
  > `"Primary evaluation metric is Mean Reciprocal Rank (MRR@10) with secondary metric Normalized Discounted Cumulative Gain (NDCG@5)."`  
  > *Authority: doc | Confidence: 0.90 | Scope: repo*
- **Fact 3 (Outlier Rejection Method):**
  > `"Signal response outliers > 3.0 interquartile ranges (IQR) above the 75th percentile are flagged as sensor artifacts and excluded."`  
  > *Authority: test | Confidence: 0.92 | Scope: repo*

#### Code AST (Symbol Signatures)
- **Analysis Pipeline Function:**  
  `def compute_cohens_d(treatment_group: np.ndarray, control_group: np.ndarray) -> float: stats/effect_size.py:32`
- **Corpus Loader Class:**  
  `class AcademicCorpusLoader: def load_parquet(self, path: Path) -> pd.DataFrame: data/corpus_reader.py:65`
- **Plotting Utility:**  
  `def generate_forest_plot(studies: List[StudyResult], output_pdf: Path) -> None: viz/forest_plot.py:110`

---

## How to Ingest Memories into Memory Hub

You can insert these memories into ContextOS using three methods:

### Method 1: The Web Dashboard (UI)
1. Open **[http://localhost:8765/](http://localhost:8765/)** in your browser.
2. Click the **"Remember"** button in the top navigation bar.
3. Select the **Kind** (`constraint`, `fact`, `decision`, `failure`), paste the **Content**, adjust **Authority** and **Confidence**, and click **Save Memory**.

### Method 2: The CLI (`ctx`)
Use the `ctx remember` command from your terminal:
```bash
./bin/ctx remember   -kind constraint   -content "Database schema changes must be backwards-compatible: column drops require a 2-phase deployment cycle."   -authority user   -confidence 1.0
```

### Method 3: Antigravity / Agent MCP Tools
When pair-programming with an agent, call the `context_remember` MCP tool:
```json
{
  "kind": "fact",
  "content": "Production Redis cluster operates on port 6379 with a 2,500ms read timeout.",
  "authority": "doc",
  "confidence": 0.95
}
```

---

## How ContextOS Code AST is Automatically Built

Unlike facts or constraints which are recorded manually or via IDE hooks, **Code AST symbols are extracted automatically** during workspace indexing:
```bash
./bin/ctx index -repo .
```
ContextOS traverses the repository, parses AST symbols (functions, types, methods, structs, interfaces), computes token footprints and line ranges, and stores them in the `nodes` and `edges` graph tables. When a prompt requires code context, ContextOS selects the exact interface signatures rather than dumping raw multi-hundred-line source files.
