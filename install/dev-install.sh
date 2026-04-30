#!/usr/bin/env bash
# Dev installer: builds Promptcellar from this checkout and registers it as a
# Claude Code plugin via the official `claude plugin marketplace add` +
# `claude plugin install` flow. (Direct cache-dir copy + installed_plugins.json
# editing does NOT work — Claude Code marks unknown-marketplace entries as
# disabled. Always go through the marketplace.)
#
# Run from anywhere; the script resolves the repo root from its own path.
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
MARKET_NAME=promptcellar
PLUGIN_NAME=promptcellar

if ! command -v claude >/dev/null 2>&1; then
  echo "claude CLI not found on PATH" >&2
  exit 1
fi

echo "==> building"
make -C "$REPO" build

# Verify the plugin manifest is valid before we ask CC to load it.
echo "==> validating plugin manifest"
claude plugin validate "$REPO/plugin"

# Add the marketplace if it's not registered. The CLI rejects bare ".";
# always pass an explicit ./relative or absolute path.
if ! claude plugin marketplace list 2>/dev/null | grep -q "^  ❯ ${MARKET_NAME}$"; then
  echo "==> registering local marketplace at $REPO"
  ( cd "$REPO" && claude plugin marketplace add ./ )
else
  echo "==> marketplace already registered, refreshing"
  claude plugin marketplace update "$MARKET_NAME" || true
fi

# Remove any prior install (handles the broken legacy promptcellar@local entry
# from the very first dev-install attempt, plus any older marketplace install).
for entry in "${PLUGIN_NAME}@local" "${PLUGIN_NAME}@${MARKET_NAME}"; do
  if claude plugin list 2>/dev/null | grep -q "^  ❯ ${entry}$"; then
    echo "==> uninstalling existing $entry"
    claude plugin uninstall "$entry" >/dev/null
  fi
done

echo "==> installing ${PLUGIN_NAME}@${MARKET_NAME}"
claude plugin install "${PLUGIN_NAME}@${MARKET_NAME}"

echo "==> claude plugin list:"
claude plugin list

cat <<EOF

✔ Installed.

Open a NEW Claude Code session in any git repo to load the plugin.
  cd /tmp/some-git-repo && claude

Inside CC:
  /promptcellar:status

The slash commands (status / enable / disable / log / doctor / version /
uninstall) are AI-driven shells over the bundled pc-cli binary. The MCP server
(pc-mcp) auto-registers via plugin/.mcp.json — agents can call
promptcellar.{search,log,touched,session}.

To uninstall: /promptcellar:uninstall  (or:  claude plugin uninstall ${PLUGIN_NAME}@${MARKET_NAME})
EOF
