#!/usr/bin/env bash
# ==============================================================================
# ContextOS: Static Dashboard Exporter for GitHub Pages
# Exports UI assets and pre-baked JSON data snapshots for zero-server hosting.
# ==============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
OUT_DIR="${1:-${ROOT_DIR}/dist}"
DATA_DIR="${OUT_DIR}/data"

echo "==> Exporting ContextOS Static Dashboard to: ${OUT_DIR}"

mkdir -p "${OUT_DIR}"
mkdir -p "${DATA_DIR}"

# 1. Copy UI frontend assets
echo "  -> Copying HTML/CSS/JS assets..."
cp -f "${ROOT_DIR}/internal/ui/assets/index.html" "${OUT_DIR}/index.html"
cp -f "${ROOT_DIR}/internal/ui/assets/style.css" "${OUT_DIR}/style.css"
cp -f "${ROOT_DIR}/internal/ui/assets/app.js" "${OUT_DIR}/app.js"

# 2. Extract Data Snapshots
PORT="${CONTEXTOS_PORT:-8765}"
LIVE_URL="http://127.0.0.1:${PORT}"

DATA_EXTRACTED=0

if curl -s -f -m 2 "${LIVE_URL}/api/status" > /dev/null 2>&1; then
    echo "  -> Live daemon detected on ${LIVE_URL}, extracting live data..."
    curl -s "${LIVE_URL}/api/status" > "${DATA_DIR}/status.json"
    curl -s "${LIVE_URL}/api/memories" > "${DATA_DIR}/memories.json"
    curl -s "${LIVE_URL}/api/sessions?all=true" > "${DATA_DIR}/sessions.json"
    curl -s "${LIVE_URL}/api/report" > "${DATA_DIR}/report.json"
    curl -s "${LIVE_URL}/api/integrations" > "${DATA_DIR}/integrations.json"
    DATA_EXTRACTED=1
else
    echo "  -> Live daemon not reachable; extracting from local storage engine..."
    
    DB_PATH=""
    if [ -f "${HOME}/.contextos/context.db" ]; then
        DB_PATH="${HOME}/.contextos/context.db"
    elif [ -f "${ROOT_DIR}/.contextos/context.db" ]; then
        DB_PATH="${ROOT_DIR}/.contextos/context.db"
    fi

    if [ -n "${DB_PATH}" ]; then
        echo "  -> Extracting from SQLite (${DB_PATH})..."
        python3 - "${DB_PATH}" "${DATA_DIR}" << 'PYEOF'
import sys, sqlite3, json

db_path = sys.argv[1]
data_dir = sys.argv[2]
conn = sqlite3.connect(db_path)
conn.row_factory = sqlite3.Row
cur = conn.cursor()

# Export memories
cur.execute("SELECT * FROM memories")
memories = [dict(r) for r in cur.fetchall()]
with open(f"{data_dir}/memories.json", "w") as f:
    json.dump(memories, f, indent=2)

# Export sessions & traces
cur.execute("SELECT * FROM sessions ORDER BY updated_at DESC")
sessions = [dict(r) for r in cur.fetchall()]
cur.execute("SELECT * FROM traces ORDER BY created_at DESC")
traces = [dict(r) for r in cur.fetchall()]
cur.execute("SELECT * FROM events ORDER BY created_at DESC LIMIT 500")
events = [dict(r) for r in cur.fetchall()]

sessions_payload = {
    "sessions": sessions,
    "traces": traces,
    "events": events,
    "models": [
        {"name": "claude-sonnet-4-6", "input_per_m": 3.0, "output_per_m": 15.0, "cached_input_per_m": 0.3},
        {"name": "gpt-5.3-codex", "input_per_m": 1.75, "output_per_m": 14.0, "cached_input_per_m": 0.175},
        {"name": "gemini-3.8-flash-high", "input_per_m": 2.0, "output_per_m": 8.0, "cached_input_per_m": 0.2}
    ],
    "session_telemetry": {
        "active_session": sessions[0]["id"] if sessions else "",
        "total_agent_events": len(events),
        "total_llm_invocations": len(events)
    }
}
with open(f"{data_dir}/sessions.json", "w") as f:
    json.dump(sessions_payload, f, indent=2)

status_payload = {
    "repo": {"name": "ContextOS", "branch": "main", "revision": "HEAD"},
    "storage": "sqlite",
    "stats": {
        "memories": len(memories),
        "decisions": sum(1 for m in memories if m.get("kind") == "decision"),
        "failures": sum(1 for m in memories if m.get("kind") == "failure")
    }
}
with open(f"{data_dir}/status.json", "w") as f:
    json.dump(status_payload, f, indent=2)

with open(f"{data_dir}/report.json", "w") as f:
    json.dump({"summary": "ContextOS Static Snapshot", "total_sessions": len(sessions)}, f)

with open(f"{data_dir}/integrations.json", "w") as f:
    json.dump([
        {"id": "antigravity", "name": "Antigravity IDE", "status": "active"},
        {"id": "claude", "name": "Claude Code", "status": "active"},
        {"id": "cursor", "name": "Cursor", "status": "active"}
    ], f)

print(f"     Successfully exported {len(memories)} memories, {len(sessions)} sessions, {len(traces)} traces.")
PYEOF
        cp -f "${DB_PATH}" "${DATA_DIR}/context.db"
        DATA_EXTRACTED=1
    elif [ -d "${ROOT_DIR}/.contextos" ] && ls "${ROOT_DIR}/.contextos/"*.json >/dev/null 2>&1; then
        echo "  -> Extracting from FileStore (.contextos/*.json)..."
        cp -f "${ROOT_DIR}/.contextos/"*.json "${DATA_DIR}/" 2>/dev/null || true
        DATA_EXTRACTED=1
    fi
fi

# 3. Fallback demo data generation if no database was found (clean CI environment)
if [ "${DATA_EXTRACTED}" -eq 0 ] || [ ! -f "${DATA_DIR}/memories.json" ]; then
    echo "  -> Seeding static demo dataset from examples/memories.example.json..."
    python3 - "${ROOT_DIR}" "${DATA_DIR}" << 'PYEOF'
import sys, os, json

root_dir = sys.argv[1]
data_dir = sys.argv[2]
example_path = os.path.join(root_dir, "examples", "memories.example.json")

memories = []
if os.path.exists(example_path):
    with open(example_path, "r") as f:
        memories = json.load(f)

with open(os.path.join(data_dir, "memories.json"), "w") as f:
    json.dump(memories, f, indent=2)

with open(os.path.join(data_dir, "status.json"), "w") as f:
    json.dump({
        "repo": {"name": "ContextOS", "branch": "main", "revision": "HEAD"},
        "storage": "file",
        "stats": {
            "memories": len(memories),
            "decisions": sum(1 for m in memories if m.get("kind") == "decision"),
            "failures": sum(1 for m in memories if m.get("kind") == "failure")
        }
    }, f, indent=2)

with open(os.path.join(data_dir, "sessions.json"), "w") as f:
    json.dump({
        "sessions": [
            {
                "id": "demo-session-1",
                "repo": "ContextOS",
                "status": "completed",
                "work_item": "wi-asc1-opt",
                "created_at": "2026-09-20T12:00:00Z",
                "updated_at": "2026-09-20T12:30:00Z"
            }
        ],
        "traces": [
            {
                "id": "tr-demo-1",
                "work_item_id": "wi-asc1-opt",
                "task": "Compress task context using ASC-1",
                "baseline_tokens": 14200,
                "allocated_tokens": 2450,
                "reduction_ratio": 0.827,
                "cost_saved": 0.035,
                "created_at": "2026-09-20T12:15:00Z"
            }
        ],
        "events": [],
        "models": [
            {"name": "claude-sonnet-4-6", "input_per_m": 3.0, "output_per_m": 15.0, "cached_input_per_m": 0.3},
            {"name": "gpt-5.3-codex", "input_per_m": 1.75, "output_per_m": 14.0, "cached_input_per_m": 0.175},
            {"name": "gemini-3.8-flash-high", "input_per_m": 2.0, "output_per_m": 8.0, "cached_input_per_m": 0.2}
        ],
        "session_telemetry": {
            "active_session": "demo-session-1",
            "total_agent_events": 12,
            "total_llm_invocations": 6
        }
    }, f, indent=2)

with open(os.path.join(data_dir, "report.json"), "w") as f:
    json.dump({"summary": "ContextOS Demo Report", "total_sessions": 1}, f, indent=2)

with open(os.path.join(data_dir, "integrations.json"), "w") as f:
    json.dump([
        {"id": "antigravity", "name": "Antigravity IDE", "status": "active"},
        {"id": "claude", "name": "Claude Code", "status": "active"},
        {"id": "cursor", "name": "Cursor", "status": "active"}
    ], f, indent=2)

print(f"     Seeded {len(memories)} sample memories into demo static dataset.")
PYEOF
fi

# 4. Inject static mode flag into index.html cross-platform via python
python3 -c "
import sys
index_path = '${OUT_DIR}/index.html'
with open(index_path, 'r') as f:
    content = f.read()
if 'window.__STATIC_MODE__' not in content:
    content = content.replace('<head>', '<head>\n  <script>window.__STATIC_MODE__ = true;</script>')
    with open(index_path, 'w') as f:
        f.write(content)
"

echo ""
echo "==> Static Dashboard Build Complete!"
echo "    Directory: ${OUT_DIR}"
echo "    Files created:"
ls -lh "${OUT_DIR}"
echo ""
echo "To preview locally:"
echo "    npx serve ${OUT_DIR}"
echo "    # or"
echo "    python3 -m http.server -d ${OUT_DIR} 8000"
echo "=============================================================================="
