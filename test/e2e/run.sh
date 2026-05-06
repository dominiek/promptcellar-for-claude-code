#!/usr/bin/env bash
# Promptcellar E2E entry point. Resolves Claude Code credentials from
# whatever the host already has, builds the test image, and runs both
# scenarios (E2E-A: curl|sh installer, E2E-B: marketplace install +
# bootstrap). Driven by `make test-e2e`.
#
# Credential resolution order — first hit wins:
#   1. ANTHROPIC_API_KEY env var          (CI / explicit override)
#   2. macOS keychain "Claude Code-credentials"   (Claude Code OAuth on macOS)
#   3. ~/.claude/.credentials.json        (Claude Code OAuth on Linux, or
#                                          manually-provisioned credential)
#
# In every case the resolved credential is passed into the container so the
# in-container `claude` authenticates the same way the host does. Local
# scratch files (for keychain extracts) live in a per-run tempdir with mode
# 0700, mounted read-only, and removed on EXIT.

set -euo pipefail

REPO=$(cd "$(dirname "$0")/../.." && pwd)
IMAGE="${PC_E2E_IMAGE:-promptcellar-e2e:latest}"

# ── credential resolution ──────────────────────────────────────────────────

DOCKER_AUTH_ARGS=()
AUTH_SOURCE=""
TMPCRED_DIR=""

cleanup() {
  if [ -n "$TMPCRED_DIR" ] && [ -d "$TMPCRED_DIR" ]; then
    rm -rf "$TMPCRED_DIR"
  fi
}
trap cleanup EXIT

if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  AUTH_SOURCE="ANTHROPIC_API_KEY env var"
  DOCKER_AUTH_ARGS=(-e ANTHROPIC_API_KEY)

elif [ "$(uname -s)" = "Darwin" ] && \
     security find-generic-password -s "Claude Code-credentials" >/dev/null 2>&1; then
  AUTH_SOURCE="macOS keychain (Claude Code-credentials)"
  TMPCRED_DIR=$(mktemp -d -t pc-e2e-cred.XXXXXX)
  chmod 700 "$TMPCRED_DIR"
  # `-w` prints just the secret to stdout. The secret is a JSON document
  # with the shape Claude Code expects in ~/.claude/.credentials.json on
  # Linux, so we can drop it in the container at that path verbatim.
  if ! security find-generic-password -s "Claude Code-credentials" -w \
        > "$TMPCRED_DIR/.credentials.json" 2>/dev/null; then
    echo "could not extract credential from keychain — try unlocking it" >&2
    exit 2
  fi
  chmod 600 "$TMPCRED_DIR/.credentials.json"
  DOCKER_AUTH_ARGS=(-v "$TMPCRED_DIR/.credentials.json:/home/tester/.claude/.credentials.json:ro")

elif [ -f "$HOME/.claude/.credentials.json" ]; then
  AUTH_SOURCE="$HOME/.claude/.credentials.json"
  DOCKER_AUTH_ARGS=(-v "$HOME/.claude/.credentials.json:/home/tester/.claude/.credentials.json:ro")

else
  cat >&2 <<EOF
No Claude Code credentials found. Tried, in order:

  1. \$ANTHROPIC_API_KEY env var (not set)
  2. macOS keychain "Claude Code-credentials" (not present, or not on macOS)
  3. ~/.claude/.credentials.json (not present)

To proceed, do one of:

  • Run \`claude login\` on this host so subsequent runs find your OAuth credential, OR
  • export ANTHROPIC_API_KEY=sk-ant-... (good for CI / non-interactive)

EOF
  exit 2
fi

echo "==> auth: ${AUTH_SOURCE}"

# ── build image (cached after first run) ────────────────────────────────────

echo "==> building image ${IMAGE}"
docker build -t "$IMAGE" -f "$REPO/test/e2e/Dockerfile" "$REPO/test/e2e" >/dev/null

# ── run scenarios ───────────────────────────────────────────────────────────

run_scenario() {
  local label=$1
  local script=$2
  echo ""
  echo "── ${label} ──────────────────────────────────────────────────────"
  docker run --rm \
    "${DOCKER_AUTH_ARGS[@]}" \
    -v "$REPO:/repo:ro" \
    "$IMAGE" \
    bash "/repo/test/e2e/${script}"
}

run_scenario "E2E-A: curl|sh installer"               run-curl-install.sh
run_scenario "E2E-B: marketplace install + bootstrap" run-marketplace-install.sh

echo ""
echo "✅ both E2E scenarios passed"
