#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs
# SPDX-License-Identifier: Apache-2.0
# ============================================================================
# smoke-remote.sh — Verify a running Scout instance (local or remote)
# ============================================================================
# Checks healthz, summary, assessments, inventory, and the HTML dashboard.
#
# Usage:
#   ./scripts/smoke-remote.sh
#   SCOUT_URL=http://175.110.122.71:18447 ./scripts/smoke-remote.sh
#
set -euo pipefail

BASE="${SCOUT_URL:-http://127.0.0.1:18447}"
BASE="${BASE%/}"

pass() { printf '  ✅ %s\n' "$*"; }
fail() { printf '  ❌ %s\n' "$*" >&2; exit 1; }

echo "Scout smoke → ${BASE}"

code="$(curl -sS -o /tmp/scout-smoke-health.json -w '%{http_code}' "${BASE}/api/v1/healthz")"
[ "$code" = "200" ] || fail "healthz HTTP ${code}"
grep -q '"ok":true\|"ok": true' /tmp/scout-smoke-health.json || fail "healthz body missing ok=true"
pass "healthz"

code="$(curl -sS -o /tmp/scout-smoke-summary.json -w '%{http_code}' "${BASE}/api/v1/summary")"
[ "$code" = "200" ] || fail "summary HTTP ${code}"
pass "summary ($(wc -c </tmp/scout-smoke-summary.json | tr -d ' ') bytes)"

code="$(curl -sS -o /tmp/scout-smoke-assessments.json -w '%{http_code}' "${BASE}/api/v1/assessments")"
[ "$code" = "200" ] || fail "assessments HTTP ${code}"
pass "assessments"

code="$(curl -sS -o /tmp/scout-smoke-inventory.json -w '%{http_code}' "${BASE}/api/v1/inventory")"
[ "$code" = "200" ] || fail "inventory HTTP ${code}"
grep -q '"vms"' /tmp/scout-smoke-inventory.json || fail "inventory missing vms"
pass "inventory"

code="$(curl -sS -o /tmp/scout-smoke-dash.html -w '%{http_code}' "${BASE}/")"
[ "$code" = "200" ] || fail "dashboard HTTP ${code}"
grep -qi 'scout\|html' /tmp/scout-smoke-dash.html || fail "dashboard body unexpected"
pass "dashboard"

echo "  ✨ smoke OK"
