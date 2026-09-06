#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# deploy-remote.sh — Deploy Scout to a remote host as a systemd service
# ============================================================================
# Scout builds as a single static binary (CGO_ENABLED=0), so deployment needs
# no remote toolchain:
#   1. Detect the remote OS/arch over SSH
#   2. Cross-compile ./cmd/scout for that target, locally
#   3. Copy the binary + inventory + env file + systemd unit
#   4. Enable/start scout.service, open the firewall port
#   5. Verify with scripts/smoke-remote.sh (health + summary + assessments)
#
# Usage:
#   ./scripts/deploy-remote.sh <host> [user] [password] [options]
#   ./scripts/deploy-remote.sh 212.8.248.187 sus
#   ./scripts/deploy-remote.sh 212.8.248.187 sus --port 19726
#   SCOUT_PORT=19726 ./scripts/deploy-remote.sh 212.8.248.187 sus
#   ./scripts/deploy-remote.sh 212.8.248.187 sus --uninstall
#   ./scripts/deploy-remote.sh 212.8.248.187 sus --dry-run
#
# Options:
#   --port N      Listen/health-check TCP port (overrides env and .deploy-last)
#   --uninstall   Stop scout.service and remove it + the binary from the host
#   --dry-run     Print what would happen; make no changes
#   --skip-smoke  Skip the smoke-remote.sh step at the end
#   --verbose     Show full remote command output
#
# Port resolution (first match wins):
#   1. --port N
#   2. SCOUT_PORT env
#   3. PORT from .deploy-last (redeploy keeps the same port)
#   4. random in 18000–28999
#
# Environment variables:
#   DEPLOY_HOST, DEPLOY_USER, DEPLOY_PASS   — same as the positional args
#   SCOUT_PORT                               — listen/health-check port
#   SCOUT_INVENTORY_SRC                      — local inventory JSON to ship
#                                               (default: sample/inventory.json)
# ============================================================================

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
# shellcheck source=lib/deploy-common.sh
source "$SCRIPT_DIR/lib/deploy-common.sh"

info()  { scout_info "$@"; }
warn()  { scout_warn "$@"; }
error() { scout_error "$@"; }
step()  { LAST_ACTION="$*"; deploy_ui_step_start "$*"; }

REMOTE_BIN=/usr/local/bin/scout
REMOTE_DIR=/etc/scout
REMOTE_ENV=/etc/scout/scout.env
REMOTE_INVENTORY=/etc/scout/inventory.json
REMOTE_UNIT=/etc/systemd/system/scout.service
SCOUT_INVENTORY_SRC="${SCOUT_INVENTORY_SRC:-$REPO_DIR/sample/inventory.json}"

# ── Parse args ──
UNINSTALL_MODE=false
DRY_RUN=false
SKIP_SMOKE=false
VERBOSE=false
PORT_FROM_CLI=""
POSITIONAL=()
while [ $# -gt 0 ]; do
    case "$1" in
        --port)
            [ $# -ge 2 ] || error "--port requires a value"
            PORT_FROM_CLI="$2"
            shift 2
            ;;
        --port=*)
            PORT_FROM_CLI="${1#*=}"
            shift
            ;;
        --uninstall)  UNINSTALL_MODE=true; shift ;;
        --dry-run)    DRY_RUN=true; shift ;;
        --skip-smoke) SKIP_SMOKE=true; shift ;;
        --verbose)    VERBOSE=true; shift ;;
        --help|-h)
            sed -n '2,48p' "$0" | sed 's/^# \{0,1\}//'
            exit 0
            ;;
        --)
            shift
            POSITIONAL+=("$@")
            break
            ;;
        -*)
            error "Unknown option: $1 (see --help)"
            ;;
        *)
            POSITIONAL+=("$1")
            shift
            ;;
    esac
done

HOST="${POSITIONAL[0]:-${DEPLOY_HOST:-}}"
USER="${POSITIONAL[1]:-${DEPLOY_USER:-root}}"
PASS="${POSITIONAL[2]:-${DEPLOY_PASS:-}}"

scout_parse_target HOST USER
LAST_PORT=""
if [ -z "$HOST" ] && scout_load_deploy_last "$REPO_DIR"; then
    info "Using .deploy-last → ${USER}@${HOST}"
    LAST_PORT="${PORT:-}"
elif [ -f "$REPO_DIR/.deploy-last" ]; then
    LAST_PORT="$(awk -F= '/^PORT=/ {print $2; exit}' "$REPO_DIR/.deploy-last")"
fi
[ -z "$HOST" ] && error "Usage: $0 <host> [user] [password] [options]  (see --help)"

# Resolve listen port: --port > env > .deploy-last > random
if [ -n "$PORT_FROM_CLI" ]; then
    SCOUT_PORT="$PORT_FROM_CLI"
elif [ -n "${SCOUT_PORT:-}" ]; then
    :
elif [ -n "$LAST_PORT" ]; then
    SCOUT_PORT="$LAST_PORT"
    info "Reusing port ${SCOUT_PORT} from .deploy-last"
else
    SCOUT_PORT=$((18000 + RANDOM % 11000))
    info "Selected random port ${SCOUT_PORT}"
fi
case "$SCOUT_PORT" in
    ''|*[!0-9]*) error "Invalid port: ${SCOUT_PORT} (need integer 1–65535)" ;;
esac
if [ "$SCOUT_PORT" -lt 1 ] || [ "$SCOUT_PORT" -gt 65535 ]; then
    error "Port out of range: ${SCOUT_PORT} (need 1–65535)"
fi

[ -f "$REPO_DIR/go.mod" ] || error "Not in the scout repo: $REPO_DIR"
[ -f "$SCOUT_INVENTORY_SRC" ] || error "Inventory not found: $SCOUT_INVENTORY_SRC (set SCOUT_INVENTORY_SRC)"
scout_build_metadata "$REPO_DIR"
DEPLOY_UI_PORT="$SCOUT_PORT"

SUDO=""
[ "$USER" != "root" ] && SUDO="sudo"

DEPLOY_SSH_OPTS=(
    -o StrictHostKeyChecking=no
    -o ConnectTimeout=15
    -o ServerAliveInterval=15
    -o ServerAliveCountMax=8
)
DEPLOY_SSH_TTY_OPTS=()
[ "$USER" != "root" ] && DEPLOY_SSH_TTY_OPTS=(-tt)

if [ -n "$PASS" ] && ! command -v sshpass &>/dev/null; then
    error "sshpass required for password auth (brew install sshpass / dnf install sshpass)"
fi

_ssh() {
    local -a ssh_args=("${DEPLOY_SSH_OPTS[@]}" "${DEPLOY_SSH_TTY_OPTS[@]}")
    if [ -n "$PASS" ]; then
        SSHPASS="$PASS" sshpass -e ssh "${ssh_args[@]}" "${USER}@${HOST}" "$@"
    else
        ssh "${ssh_args[@]}" "${USER}@${HOST}" "$@"
    fi
}

_ssh_batch() {
    local -a ssh_args=("${DEPLOY_SSH_OPTS[@]}")
    if [ -n "$PASS" ]; then
        SSHPASS="$PASS" sshpass -e ssh "${ssh_args[@]}" "${USER}@${HOST}" "$@"
    else
        ssh "${ssh_args[@]}" "${USER}@${HOST}" "$@"
    fi
}

_scp() {
    local -a scp_args=("${DEPLOY_SSH_OPTS[@]}")
    if [ -n "$PASS" ]; then
        SSHPASS="$PASS" sshpass -e scp "${scp_args[@]}" "$@"
    else
        scp "${scp_args[@]}" "$@"
    fi
}

if $DRY_RUN; then
    deploy_ui_banner "${DEPLOY_UI_ICON_MAGIC} Dry run" "no changes will be made"
    deploy_ui_kv "🎯" "Target" "${USER}@${HOST}"
    deploy_ui_kv "📦" "Binary" "$REMOTE_BIN"
    deploy_ui_kv "📄" "Inventory" "$REMOTE_INVENTORY"
    deploy_ui_kv "📄" "Env file" "$REMOTE_ENV (written only if missing)"
    deploy_ui_kv "⚙️" "Unit" "$REMOTE_UNIT"
    echo ""
    deploy_ui_note "Would: detect arch → cross-compile locally → scp binary+inventory+unit → enable/start → smoke-remote.sh"
    echo ""
    exit 0
fi

deploy_ui_banner "Remote Deploy" "${SCOUT_GIT_VERSION} (${SCOUT_GIT_COMMIT}) → ${USER}@${HOST}"
deploy_ui_kv "🎯" "Target" "${USER}@${HOST}"
deploy_ui_kv "🔐" "Auth" "$([ -n "$PASS" ] && echo 'password' || echo 'SSH key')"
deploy_ui_kv "🌐" "Port" "$SCOUT_PORT"
deploy_ui_kv "📋" "Inventory" "$SCOUT_INVENTORY_SRC"
echo ""

# ── Uninstall mode ──
if $UNINSTALL_MODE; then
    deploy_ui_uninstall_banner
    step "Uninstalling scout from ${HOST}"
    _ssh "
        $SUDO systemctl disable --now scout.service 2>/dev/null || true
        $SUDO rm -f $REMOTE_BIN $REMOTE_UNIT
        $SUDO rm -rf /etc/scout
        $SUDO systemctl daemon-reload 2>/dev/null || true
    "
    info "scout removed from ${HOST}"
    exit 0
fi

# ── Step 1: detect remote OS/arch ──
step "Detecting remote architecture"
REMOTE_ARCH_RAW=$(_ssh_batch "uname -m" | tr -d '\r')
case "$REMOTE_ARCH_RAW" in
    x86_64)         GOARCH=amd64 ;;
    aarch64|arm64)  GOARCH=arm64 ;;
    *) error "Unsupported remote architecture: $REMOTE_ARCH_RAW" ;;
esac
info "Remote: linux/${GOARCH}"

# ── Step 2: cross-compile locally ──
step "Cross-compiling scout for linux/${GOARCH}"
BUILD_DIR="$(mktemp -d)"
trap 'rm -rf "$BUILD_DIR"' EXIT
(
    cd "$REPO_DIR"
    CGO_ENABLED=0 GOOS=linux GOARCH="$GOARCH" \
        go build -trimpath -ldflags="-s -w" -o "$BUILD_DIR/scout" ./cmd/scout
)
info "Built $BUILD_DIR/scout"

# ── Step 3: install binary + inventory + env + unit ──
step "Installing scout on ${HOST}"
_ssh "$SUDO mkdir -p $REMOTE_DIR"
_scp "$BUILD_DIR/scout" "${USER}@${HOST}:/tmp/scout.new"
_scp "$SCOUT_INVENTORY_SRC" "${USER}@${HOST}:/tmp/scout.inventory.new"
_ssh "
    $SUDO install -m 755 /tmp/scout.new $REMOTE_BIN && rm -f /tmp/scout.new
    $SUDO install -m 644 /tmp/scout.inventory.new $REMOTE_INVENTORY && rm -f /tmp/scout.inventory.new
"

cat > "$BUILD_DIR/scout.env" <<ENVEOF
SCOUT_ADDR=0.0.0.0:${SCOUT_PORT}
SCOUT_INVENTORY=${REMOTE_INVENTORY}
ENVEOF
_scp "$BUILD_DIR/scout.env" "${USER}@${HOST}:/tmp/scout.env.new"
# Refresh listen address on every deploy so SCOUT_PORT changes take effect.
# Custom keys beyond SCOUT_ADDR / SCOUT_INVENTORY are not preserved in v0.1.
_ssh "
    $SUDO install -m 640 /tmp/scout.env.new $REMOTE_ENV
    rm -f /tmp/scout.env.new
"

_scp "$REPO_DIR/systemd/scout.service" "${USER}@${HOST}:/tmp/scout.service.new"
_ssh "$SUDO install -m 644 /tmp/scout.service.new $REMOTE_UNIT && rm -f /tmp/scout.service.new"
info "Binary + inventory + config + systemd unit installed"

# ── Step 4: enable/start service, open firewall ──
step "Starting scout.service"
_ssh "
    $SUDO systemctl daemon-reload
    $SUDO systemctl enable --now scout.service
    $SUDO systemctl restart scout.service
    if command -v firewall-cmd &>/dev/null; then
        $SUDO firewall-cmd --permanent --add-port=${SCOUT_PORT}/tcp 2>/dev/null || true
        $SUDO firewall-cmd --reload 2>/dev/null || true
    elif command -v ufw &>/dev/null; then
        $SUDO ufw allow ${SCOUT_PORT}/tcp 2>/dev/null || true
    fi
    sleep 1
    if $SUDO systemctl is-active scout.service &>/dev/null; then
        echo 'scout.service: running'
    else
        echo 'scout.service: FAILED TO START'
        $SUDO journalctl -u scout.service --no-pager -n 20
        exit 1
    fi
"
info "scout.service active"

# ── Step 5: verify ──
step "Verifying deployment"
BASE_URL="http://${HOST}:${SCOUT_PORT}"
DEPLOY_UI_SCHEME="http"
_ssh "curl -fsS http://127.0.0.1:${SCOUT_PORT}/api/v1/healthz >/dev/null" \
    && info "Health check OK (http://127.0.0.1:${SCOUT_PORT}/api/v1/healthz, on-host)"

scout_save_deploy_last "$REPO_DIR" "$HOST" "$USER" "full"

deploy_ui_highlight "📋 Final checklist"
deploy_ui_checklist "service" "$(_ssh_batch "$SUDO systemctl is-active scout.service" | tr -d '\r')"
deploy_ui_checklist "health"  "$(_ssh_batch "curl -s -o /dev/null -w '%{http_code}' http://127.0.0.1:${SCOUT_PORT}/api/v1/healthz" | tr -d '\r')"

scout_print_success "$HOST" 0

# ── Step 6: smoke from the workstation against the remote URL ──
if $SKIP_SMOKE; then
    info "Skipped smoke-remote.sh (--skip-smoke)"
else
    step "Running scripts/smoke-remote.sh against ${BASE_URL}"
    ( cd "$REPO_DIR" && SCOUT_URL="$BASE_URL" ./scripts/smoke-remote.sh )
fi
