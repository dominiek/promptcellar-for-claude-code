#!/usr/bin/env sh
# Promptcellar for Claude Code — production installer.
#
# Run via:   curl -fsSL https://get.promptcellar.io/claude-code | sh
#
# Adds the public marketplace and installs the plugin via Claude Code's own
# `claude plugin install` flow — the only path that produces a working install
# (direct cache-dir + installed_plugins.json edits result in disabled-status
# plugins).

set -eu

MARKETPLACE_REPO="${PC_MARKETPLACE:-dominiek/promptcellar-for-claude-code}"
MARKET_NAME="${PC_MARKET_NAME:-promptcellar}"
PLUGIN_NAME="${PC_PLUGIN_NAME:-promptcellar}"

if ! command -v claude >/dev/null 2>&1; then
  echo "Claude Code CLI ('claude') not found on PATH." >&2
  echo "Install Claude Code first: https://docs.claude.com/code" >&2
  exit 1
fi

if claude plugin marketplace list 2>/dev/null | grep -q "^  ❯ ${MARKET_NAME}$"; then
  echo "==> marketplace ${MARKET_NAME} already registered"
  claude plugin marketplace update "${MARKET_NAME}" || true
else
  echo "==> adding marketplace ${MARKETPLACE_REPO}"
  claude plugin marketplace add "${MARKETPLACE_REPO}"
fi

if claude plugin list 2>/dev/null | grep -q "^  ❯ ${PLUGIN_NAME}@${MARKET_NAME}$"; then
  echo "==> already installed; updating"
  claude plugin update "${PLUGIN_NAME}@${MARKET_NAME}" || true
else
  echo "==> installing ${PLUGIN_NAME}@${MARKET_NAME}"
  claude plugin install "${PLUGIN_NAME}@${MARKET_NAME}"
fi

cat <<'EOF'

✔ Installed.

Open a NEW Claude Code session in any git repo:
  cd /path/to/your/repo && claude

Inside CC:
  /promptcellar:status   # confirm capture is on
  /promptcellar:log      # see captured prompts
  /promptcellar:disable  # opt out for this repo

EOF
