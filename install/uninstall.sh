#!/usr/bin/env sh
# Promptcellar for Claude Code — uninstaller.
#
# Run via:   curl -fsSL https://get.promptcellar.io/claude-code/uninstall | sh
#
# Removes the plugin via Claude Code's own `claude plugin uninstall` flow and
# (by default) drops the marketplace registration. Captured .prompts/ data in
# your repos is left untouched.
#
# Env:
#   PC_KEEP_MARKETPLACE=1   leave the marketplace registered after uninstall

set -eu

MARKETPLACE_REPO="${PC_MARKETPLACE:-dominiek/promptcellar-for-claude-code}"
MARKET_NAME="${PC_MARKET_NAME:-promptcellar}"
PLUGIN_NAME="${PC_PLUGIN_NAME:-promptcellar}"

if ! command -v claude >/dev/null 2>&1; then
  echo "Claude Code CLI ('claude') not found on PATH." >&2
  echo "Nothing to uninstall." >&2
  exit 1
fi

if claude plugin list 2>/dev/null | grep -q "^  ❯ ${PLUGIN_NAME}@${MARKET_NAME}$"; then
  echo "==> uninstalling ${PLUGIN_NAME}@${MARKET_NAME}"
  claude plugin uninstall "${PLUGIN_NAME}@${MARKET_NAME}"
else
  echo "==> ${PLUGIN_NAME}@${MARKET_NAME} not installed; skipping"
fi

if [ "${PC_KEEP_MARKETPLACE:-0}" = "1" ]; then
  echo "==> keeping marketplace ${MARKET_NAME} (PC_KEEP_MARKETPLACE=1)"
elif claude plugin marketplace list 2>/dev/null | grep -q "^  ❯ ${MARKET_NAME}$"; then
  echo "==> removing marketplace ${MARKET_NAME}"
  claude plugin marketplace remove "${MARKET_NAME}" || true
else
  echo "==> marketplace ${MARKET_NAME} not registered; skipping"
fi

cat <<'EOF'

✔ Uninstalled.

Captured .prompts/ data in your repos is left intact. To remove it from a
specific repo:
  rm -rf /path/to/repo/.prompts

To reinstall later:
  curl -fsSL https://get.promptcellar.io/claude-code | sh

EOF
