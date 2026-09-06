#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs
# SPDX-License-Identifier: Apache-2.0
# ============================================================================
# smoke-remote.sh — Verify a running Scout instance (local or remote)
# ============================================================================
# Checks healthz, summary, assessments, inventory, and the HTML dashboard.
#
# Usage:
#   SCOUT_URL=http://212.8.248.187:19726 ./scripts/smoke-remote.sh
#   ./scripts/smoke-remote.sh --port 19726
#   SCOUT_PORT=19726 ./scripts/smoke-remote.sh
#   ./scripts/smoke-remote.sh   # uses HOST:PORT from .deploy-last
#
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
PORT_FROM_CLI=""
while [ $# -gt 0 ]; do
  case "$1" in
    --port) [ $# -ge 2 ] || { echo "--port requires a value" >&2; exit 2; }; PORT_FROM_CLI="$2"; shift 2 ;;
    --port=*) PORT_FROM_CLI="${1#*=}"; shift ;;
    --help|-h)
      sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "Unknown option: $1" >&2; exit 2 ;;
  esac
done

BASE="${SCOUT_URL:-}"
HOST_FROM_LAST=""
PORT_FROM_LAST=""
if [ -f "$ROOT/.deploy-last" ]; then
  # shellcheck disable=SC1091
  source "$ROOT/.deploy-last"
  HOST_FROM_LAST="${HOST:-}"
  PORT_FROM_LAST="${PORT:-}"
fi

if [ -z "$BASE" ]; then
  PORT_RESOLVED="${PORT_FROM_CLI:-${SCOUT_PORT:-$PORT_FROM_LAST}}"
  HOST_RESOLVED="${SCOUT_HOST:-$HOST_FROM_LAST}"
  if [ -n "$HOST_RESOLVED" ] && [ -n "$PORT_RESOLVED" ]; then
    BASE="http://${HOST_RESOLVED}:${PORT_RESOLVED}"
  elif [ -n "$PORT_RESOLVED" ]; then
    BASE="http://127.0.0.1:${PORT_RESOLVED}"
  fi
fi
[ -n "$BASE" ] || {
  echo "Set SCOUT_URL=http://host:port, or --port / SCOUT_PORT with host from .deploy-last" >&2
  exit 2
}
BASE="${BASE%/}"
TMPDIR_SMOKE="${TMPDIR:-/tmp}"

pass() { printf '  ✅ %s\n' "$*"; }
fail() { printf '  ❌ %s\n' "$*" >&2; exit 1; }

echo "Scout smoke → ${BASE}"

code="$(curl -sS -o "${TMPDIR_SMOKE}/scout-smoke-health.json" -w '%{http_code}' "${BASE}/api/v1/healthz")"
[ "$code" = "200" ] || fail "healthz HTTP ${code}"
grep -q '"ok":true\|"ok": true' "${TMPDIR_SMOKE}/scout-smoke-health.json" || fail "healthz body missing ok=true"
pass "healthz"

code="$(curl -sS -o "${TMPDIR_SMOKE}/scout-smoke-summary.json" -w '%{http_code}' "${BASE}/api/v1/summary")"
[ "$code" = "200" ] || fail "summary HTTP ${code}"
pass "summary ($(wc -c <"${TMPDIR_SMOKE}/scout-smoke-summary.json" | tr -d ' ') bytes)"

code="$(curl -sS -o "${TMPDIR_SMOKE}/scout-smoke-assessments.json" -w '%{http_code}' "${BASE}/api/v1/assessments")"
[ "$code" = "200" ] || fail "assessments HTTP ${code}"
pass "assessments"

code="$(curl -sS -o "${TMPDIR_SMOKE}/scout-smoke-inventory.json" -w '%{http_code}' "${BASE}/api/v1/inventory")"
[ "$code" = "200" ] || fail "inventory HTTP ${code}"
grep -q '"vms"' "${TMPDIR_SMOKE}/scout-smoke-inventory.json" || fail "inventory missing vms"
pass "inventory"

code="$(curl -sS -o "${TMPDIR_SMOKE}/scout-smoke-dash.html" -w '%{http_code}' "${BASE}/")"
[ "$code" = "200" ] || fail "dashboard HTTP ${code}"
grep -qi 'scout\|html' "${TMPDIR_SMOKE}/scout-smoke-dash.html" || fail "dashboard body unexpected"
pass "dashboard"

echo "  ✨ smoke OK"
