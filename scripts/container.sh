#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs
# SPDX-License-Identifier: Apache-2.0
# ============================================================================
# container.sh — Build and run Scout with Podman or Docker
# ============================================================================
# Usage:
#   ./scripts/container.sh up [--build]
#   ./scripts/container.sh down
#   ./scripts/container.sh build
#   ./scripts/container.sh run [inventory.json]
#   ./scripts/container.sh smoke
#   ./scripts/container.sh logs
#   ./scripts/container.sh which
#
# Environment:
#   CTR=podman|docker          force runtime (auto-detects otherwise)
#   COMPOSE='podman compose'   force compose frontend
#   IMAGE=ghcr.io/zyvorai/scout:0.1.0
#   SCOUT_PORT=18447
#   SCOUT_INVENTORY_SRC=./sample/inventory.json
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=lib/container.sh
source "$SCRIPT_DIR/lib/container.sh"

IMAGE="${IMAGE:-ghcr.io/zyvorai/scout:0.1.0}"
SCOUT_PORT="${SCOUT_PORT:-18447}"
SCOUT_INVENTORY_SRC="${SCOUT_INVENTORY_SRC:-$REPO_DIR/sample/inventory.json}"
NAME="${SCOUT_CONTAINER_NAME:-scout}"

usage() {
    sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'
}

cmd="${1:-}"
shift || true

case "$cmd" in
    which)
        if ! container_detect; then
            echo "CTR=(none — install podman or docker)"
            exit 1
        fi
        if compose_detect; then
            echo "CTR=$CTR"
            echo "COMPOSE=${COMPOSE_CMD[*]}"
        else
            echo "CTR=$CTR"
            echo "COMPOSE=(unavailable)"
        fi
        echo "IMAGE=$IMAGE"
        ;;
    build)
        container_detect
        echo "Building $IMAGE with $CTR"
        ( cd "$REPO_DIR" && "$CTR" build -t "$IMAGE" . )
        ;;
    up)
        compose_detect
        extra=()
        for a in "$@"; do
            case "$a" in
                --build) extra+=(--build) ;;
                *) echo "unknown arg: $a" >&2; exit 2 ;;
            esac
        done
        echo "Starting with ${COMPOSE_CMD[*]}"
        ( cd "$REPO_DIR" && "${COMPOSE_CMD[@]}" up -d "${extra[@]}" )
        echo "Dashboard → http://127.0.0.1:${SCOUT_PORT}/"
        ;;
    down)
        compose_detect
        ( cd "$REPO_DIR" && "${COMPOSE_CMD[@]}" down )
        ;;
    logs)
        compose_detect
        ( cd "$REPO_DIR" && "${COMPOSE_CMD[@]}" logs -f )
        ;;
    run)
        container_detect
        inv="${1:-$SCOUT_INVENTORY_SRC}"
        [ -f "$inv" ] || { echo "inventory not found: $inv" >&2; exit 1; }
        abs="$(cd "$(dirname "$inv")" && pwd)/$(basename "$inv")"
        "$CTR" rm -f "$NAME" >/dev/null 2>&1 || true
        "$CTR" run -d --name "$NAME" \
            --read-only \
            --tmpfs /tmp:rw,size=16m \
            -p "${SCOUT_PORT}:18447" \
            -v "${abs}:/data/inventory.json:ro" \
            "$IMAGE" \
            serve --file /data/inventory.json --addr :18447
        echo "Running ($CTR) → http://127.0.0.1:${SCOUT_PORT}/"
        ;;
    smoke)
        SCOUT_URL="${SCOUT_URL:-http://127.0.0.1:${SCOUT_PORT}}" "$REPO_DIR/scripts/smoke-remote.sh"
        ;;
    stop)
        container_detect
        "$CTR" stop "$NAME" >/dev/null 2>&1 || true
        "$CTR" rm -f "$NAME" >/dev/null 2>&1 || true
        echo "stopped $NAME"
        ;;
    help|-h|--help|"")
        usage
        [ -n "$cmd" ] || exit 2
        ;;
    *)
        echo "unknown command: $cmd" >&2
        usage
        exit 2
        ;;
esac
