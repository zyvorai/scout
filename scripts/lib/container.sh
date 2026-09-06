#!/usr/bin/env bash
# Copyright 2026 Zyvor AI Labs · https://zyvor.dev
# SPDX-License-Identifier: Apache-2.0
# shellcheck shell=bash
# Shared container-runtime detection for Docker and Podman.

container_detect() {
    if [ -n "${CTR:-}" ]; then
        return 0
    fi
    if command -v podman >/dev/null 2>&1; then
        CTR=podman
    elif command -v docker >/dev/null 2>&1; then
        CTR=docker
    else
        echo "error: need podman or docker in PATH" >&2
        return 1
    fi
}

compose_detect() {
    container_detect || return 1
    if [ -n "${COMPOSE:-}" ]; then
        # COMPOSE is a string of words (e.g. "podman compose")
        # shellcheck disable=SC2206
        COMPOSE_CMD=($COMPOSE)
        return 0
    fi
    case "$CTR" in
        podman)
            if podman compose version >/dev/null 2>&1; then
                COMPOSE_CMD=(podman compose)
            elif command -v podman-compose >/dev/null 2>&1; then
                COMPOSE_CMD=(podman-compose)
            else
                echo "error: podman found but neither 'podman compose' nor podman-compose is available" >&2
                return 1
            fi
            ;;
        docker)
            if docker compose version >/dev/null 2>&1; then
                COMPOSE_CMD=(docker compose)
            elif command -v docker-compose >/dev/null 2>&1; then
                COMPOSE_CMD=(docker-compose)
            else
                echo "error: docker found but Compose v2/v1 is not available" >&2
                return 1
            fi
            ;;
        *)
            echo "error: unsupported CTR=$CTR" >&2
            return 1
            ;;
    esac
}
