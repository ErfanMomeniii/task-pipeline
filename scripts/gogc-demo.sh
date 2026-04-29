#!/usr/bin/env bash
# gogc-demo.sh — Demonstrates GOGC and GOMEMLIMIT impact on the consumer service.
#
# Usage:
#   ./scripts/gogc-demo.sh
#
# Prerequisites:
#   - Docker compose stack running (make up)
#   - curl and jq available
#
# This script restarts the consumer container with different GOGC/GOMEMLIMIT
# settings and captures heap profiles for comparison.

set -euo pipefail

COMPOSE_FILE="deploy/docker-compose.yml"
CONSUMER_PPROF="http://localhost:6061"
PROFILE_DIR="profiles/gogc-demo"

mkdir -p "$PROFILE_DIR"

collect_heap() {
    local label="$1"
    echo "  Waiting 10s for tasks to accumulate..."
    sleep 10
    echo "  Collecting heap profile → ${PROFILE_DIR}/${label}.heap.pb.gz"
    curl -s "${CONSUMER_PPROF}/debug/pprof/heap" -o "${PROFILE_DIR}/${label}.heap.pb.gz"

    # Grab current memory stats from /debug/pprof/heap?debug=1
    echo "  Memory stats:"
    curl -s "${CONSUMER_PPROF}/debug/pprof/heap?debug=1" | head -30 | grep -E '(Alloc|Sys|HeapInuse|NumGC|NextGC)' || true
    echo ""
}

echo "================================================"
echo "GOGC / GOMEMLIMIT Demo"
echo "================================================"
echo ""

# --- Run 1: Default (GOGC=100, no GOMEMLIMIT) ---
echo "[1/3] Default settings (GOGC=100, no GOMEMLIMIT)"
echo "  Restarting consumer with defaults..."
docker compose -f "$COMPOSE_FILE" rm -sf consumer >/dev/null 2>&1
docker compose -f "$COMPOSE_FILE" up -d consumer >/dev/null 2>&1
collect_heap "default"

# --- Run 2: Aggressive GC (GOGC=50) — more frequent GC, lower memory ---
echo "[2/3] Aggressive GC (GOGC=50)"
echo "  Restarting consumer with GOGC=50..."
docker compose -f "$COMPOSE_FILE" rm -sf consumer >/dev/null 2>&1
GOGC=50 docker compose -f "$COMPOSE_FILE" up -d consumer >/dev/null 2>&1
collect_heap "gogc50"

# --- Run 3: Memory limit (GOMEMLIMIT=64MiB, GOGC=off) ---
echo "[3/3] Memory-limited (GOMEMLIMIT=64MiB, GOGC=off)"
echo "  Restarting consumer with GOMEMLIMIT=64MiB, GOGC=off..."
docker compose -f "$COMPOSE_FILE" rm -sf consumer >/dev/null 2>&1
GOMEMLIMIT=67108864 GOGC=off docker compose -f "$COMPOSE_FILE" up -d consumer >/dev/null 2>&1
collect_heap "memlimit64"

# --- Restore defaults ---
echo "Restoring consumer with default settings..."
docker compose -f "$COMPOSE_FILE" rm -sf consumer >/dev/null 2>&1
docker compose -f "$COMPOSE_FILE" up -d consumer >/dev/null 2>&1

echo ""
echo "================================================"
echo "Profiles saved in ${PROFILE_DIR}/"
echo ""
echo "Compare heap profiles:"
echo "  go tool pprof -http=:8090 ${PROFILE_DIR}/default.heap.pb.gz"
echo "  go tool pprof -http=:8091 ${PROFILE_DIR}/gogc50.heap.pb.gz"
echo "  go tool pprof -http=:8092 ${PROFILE_DIR}/memlimit64.heap.pb.gz"
echo ""
echo "Key observations:"
echo "  - GOGC=50: GC runs ~2x more often → lower peak heap, higher CPU"
echo "  - GOMEMLIMIT+GOGC=off: GC only runs when approaching limit → predictable memory ceiling"
echo "  - Default: balanced trade-off between memory and CPU"
echo "================================================"
