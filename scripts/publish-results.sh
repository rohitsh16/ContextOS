#!/usr/bin/env bash
set -euo pipefail

# ContextOS Benchmark & Evaluation Publisher
# Generates publication-ready BENCHMARK_REPORT.md and results.json from local runs.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

echo "====================================================="
echo "   ContextOS Research & Benchmark Report Generator   "
echo "====================================================="

if [[ ! -f "./bin/ctx" ]]; then
  echo "Building ContextOS CLI binary..."
  CC=/usr/bin/clang go build -o bin/ctx ./cmd/ctx
fi

OUTPUT_MD="${1:-BENCHMARK_REPORT.md}"
OUTPUT_JSON="${2:-results.json}"

echo "Generating Markdown Benchmark Report -> ${OUTPUT_MD}..."
./bin/ctx publish -repo . -output "${OUTPUT_MD}"

echo "Generating JSON Telemetry Report     -> ${OUTPUT_JSON}..."
./bin/ctx report -repo . -format json -output "${OUTPUT_JSON}"

echo ""
echo "Successfully published benchmark evaluation artifacts:"
echo "  1. Markdown Report: ${REPO_ROOT}/${OUTPUT_MD}"
echo "  2. JSON Telemetry : ${REPO_ROOT}/${OUTPUT_JSON}"
echo ""
echo "Quick preview of executive summary:"
head -n 26 "${OUTPUT_MD}"
echo "====================================================="
