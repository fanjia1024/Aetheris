#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
COMPOSE_FILE="${COMPOSE_FILE:-$ROOT_DIR/deployments/compose/docker-compose.yml}"

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: required command not found: $1" >&2
    exit 1
  fi
}

compose_cmd() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    docker compose -f "$COMPOSE_FILE" "$@"
    return
  fi
  if command -v docker-compose >/dev/null 2>&1; then
    docker-compose -f "$COMPOSE_FILE" "$@"
    return
  fi
  echo "error: neither 'docker compose' nor 'docker-compose' is available" >&2
  exit 1
}

usage() {
  cat <<USAGE
Usage: scripts/local-2.0-stack.sh <command>

Commands:
  start    Build and start local 2.0 stack (postgres + api + worker1 + worker2)
  stop     Stop and remove local 2.0 stack
  status   Show running container status
  logs     Tail logs for all stack services
  health   Call API health endpoint (http://localhost:8080/api/health)
USAGE
}

cmd_start() {
  require_cmd curl
  echo "[stack] starting local 2.0 stack..."
  compose_cmd up -d --build
  echo "[stack] waiting for API health..."
  for i in {1..30}; do
    if curl -fsS "http://localhost:8080/api/health" >/dev/null 2>&1; then
      echo "[stack] API is healthy"
      compose_cmd ps
      return
    fi
    sleep 1
  done
  echo "[stack] API health check timed out" >&2
  compose_cmd ps || true
  exit 1
}

cmd_stop() {
  echo "[stack] stopping local 2.0 stack..."
  compose_cmd down
  echo "[stack] stopped"
}

cmd_status() {
  compose_cmd ps
}

cmd_logs() {
  compose_cmd logs -f
}

cmd_health() {
  require_cmd curl
  curl -fsS "http://localhost:8080/api/health"
  echo
}

main() {
  if [[ $# -lt 1 ]]; then
    usage
    exit 1
  fi

  case "$1" in
    start)
      cmd_start
      ;;
    stop)
      cmd_stop
      ;;
    status)
      cmd_status
      ;;
    logs)
      cmd_logs
      ;;
    health)
      cmd_health
      ;;
    -h|--help|help)
      usage
      ;;
    *)
      usage
      exit 1
      ;;
  esac
}

main "$@"
