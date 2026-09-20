# Deploying ContextOS Dashboard to GitHub Pages (Static Hosting)

This guide details how to deploy the **ContextOS Dashboard** as a serverless static web application on **GitHub Pages**, enabling teams, stakeholders, and open-source contributors to inspect AI agent context efficiency, cost savings, and the Memory Hub without running a live Go daemon.

---

## 1. Architectural Blueprint

The ContextOS dashboard (`internal/ui/assets/`) is engineered with **Vanilla HTML, CSS, and Modern JavaScript** (`index.html`, `style.css`, `app.js`). It contains no server-side templating. 

When hosted on GitHub Pages, the Go web server is omitted. Instead, data is served statically through one of two architectures:

```
                  ┌────────────────────────────────────────────────────────┐
                  │              GitHub Pages Static Hosting               │
                  │                                                        │
                  │   index.html  +  style.css  +  app.js (Dual-Mode)      │
                  └───────────────┬────────────────────────┬───────────────┘
                                  │                        │
                      [Option A: FileStore]    [Option B: SQLite WASM]
                                  │                        │
                                  ▼                        ▼
                          ./data/*.json               ./context.db
                     (Pre-exported Snapshots)     (Queried via sql.js)
```

| Dimension | Option A: FileStore JSON Snapshots (Recommended) | Option B: In-Browser SQLite (WASM) |
| :--- | :--- | :--- |
| **Data Format** | Static `.json` files in `./data/` | Raw portable `context.db` file |
| **Client Engine** | Native browser `fetch()` | `sql.js` (SQLite compiled to WebAssembly) |
| **Bandwidth** | Lightweight (5–50 KB per request) | ~500 KB WASM + size of `context.db` |
| **Interactivity** | Pre-filtered cards, sessions, and telemetry | Full client-side SQL execution & dynamic filtering |
| **Mobile Speed** | Instant load, 0ms WASM compilation overhead | Moderate load time on low-end mobile devices |

---

## 2. Option A: FileStore JSON Snapshots (Recommended)

### Directory Layout on GitHub Pages (`gh-pages` branch or `docs/`)
```
dist-dashboard/
├── index.html                  # Dashboard frontend structure
├── style.css                   # Theme and layout styles
├── app.js                      # Dual-mode dashboard client
└── data/                       # Static FileStore export
    ├── status.json             # System info, total memories, commit hash
    ├── memories.json           # Active Memory Hub items (Constraints, Facts, ASTs)
    ├── sessions.json           # Agent sessions, traces, and model pricing telemetry
    ├── report.json             # ASC-1 benchmark savings and latency report
    └── integrations.json       # Supported IDE & agent connectors
```

### Dual-Mode Logic in `app.js`
The client checks if it is being served from GitHub Pages or a static host, seamlessly adapting endpoint paths:

```javascript
// Automatically detect static GitHub Pages hosting
const isStatic = window.location.hostname.endsWith('github.io') || 
                 window.location.protocol === 'file:' || 
                 window.__STATIC_MODE__ === true;

async function apiGet(endpoint) {
  if (isStatic) {
    // Convert "/api/sessions?all=true" -> "./data/sessions.json"
    const file = endpoint.replace(/^\/api\//, '').split('?')[0];
    const res = await fetch(`./data/${file}.json`);
    if (res.ok) return res;
  }
  return fetch(endpoint);
}
```

---

## 3. Option B: In-Browser SQLite via WebAssembly (`sql.js`)

If you want visitors to query the actual `context.db` directly in their browser:

1. Add `sql.js` to `index.html`:
   ```html
   <script src="https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/sql-wasm.js"></script>
   ```

2. Load `context.db` into memory via WebAssembly:
   ```javascript
   async function initDatabase() {
     const SQL = await initSqlJs({
       locateFile: file => `https://cdnjs.cloudflare.com/ajax/libs/sql.js/1.10.3/${file}`
     });

     const response = await fetch('./data/context.db');
     const buffer = await response.arrayBuffer();
     const db = new SQL.Database(new Uint8Array(buffer));

     // Execute SQL queries client-side with zero backend server
     const sessions = db.exec("SELECT * FROM sessions ORDER BY updated_at DESC");
     const memories = db.exec("SELECT * FROM memories WHERE status = 'active'");
     return { sessions, memories };
   }
   ```

---

## 4. Automated Export Script (`scripts/export_static_dashboard.sh`)

ContextOS provides an automated script to build the static dashboard distribution from your current environment:

```bash
# Build static dashboard package into dist-dashboard/
./scripts/export_static_dashboard.sh ./dist-dashboard

# Test preview locally
npx serve ./dist-dashboard
# or
python3 -m http.server -d ./dist-dashboard 3000
```

---

## 5. GitHub Actions Workflow

Add `.github/workflows/deploy-dashboard.yml` to automatically rebuild and publish the latest metrics to GitHub Pages on every push:

```yaml
name: Deploy ContextOS Static Dashboard to GitHub Pages

on:
  push:
    branches: [ main ]
    paths:
      - '.contextos/**'
      - 'internal/ui/**'
  schedule:
    - cron: '0 0 * * *' # Daily automated refresh
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: "pages-dashboard"
  cancel-in-progress: false

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Generate Static Export
        run: |
          chmod +x scripts/export_static_dashboard.sh
          ./scripts/export_static_dashboard.sh ./dist-dashboard

      - name: Upload Pages Artifact
        uses: actions/upload-pages-artifact@v3
        with:
          path: ./dist-dashboard

  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    runs-on: ubuntu-latest
    needs: build
    steps:
      - name: Deploy to GitHub Pages
        id: deployment
        uses: actions/deploy-pages@v4
```

---

## 6. Privacy & Data Sanitization Checklist

Before deploying agent traces and memories to a public GitHub Pages site:

1. **Scrub Credentials**: Ensure environment variables (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GITHUB_TOKEN`) are never present in memory contents or tool arguments.
2. **Sanitize Paths**: The exporter automatically normalizes absolute filesystem paths (`/Users/username/...` -> `repo/...`).
3. **Redact Sensitive Prompts**: If your agent sessions contain proprietary user prompts, filter sessions or set `"authority": "internal"` before exporting.
