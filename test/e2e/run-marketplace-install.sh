#!/usr/bin/env bash
# E2E-B: install Promptcellar via Claude Code's `/plugin install` flow ONLY,
# with no separate binary fetch — proves the SessionStart bootstrap shim
# (added in PR #14) actually fetches the platform binary on first session.
#
# This is the path the official Anthropic plugin marketplace uses, and the
# one a user gets when they run `/plugin install promptcellar@<marketplace>`
# in-session. Before PR #14 this route looked like it worked (install
# reported success) but every hook silently failed because plugin/bin/ was
# gitignored. This test exists to catch any regression of that bug.
#
# Runs INSIDE the test container. Same env as run-curl-install.sh.

set -euo pipefail

PROMPT="${EXPECTED_PROMPT:-what time is it}"
MARKETPLACE_REPO="${PC_MARKETPLACE:-dominiek/promptcellar-for-claude-code}"
MARKET_NAME="${PC_MARKET_NAME:-promptcellar}"
PLUGIN_NAME="${PC_PLUGIN_NAME:-promptcellar}"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo 'ANTHROPIC_API_KEY not set — required for `claude -p` round trip' >&2
  exit 2
fi

cd /home/tester/work

echo "==> [E2E-B] preparing fresh /work git repo"
rm -rf .prompts .promptcellar .git ./* ./.[!.]* 2>/dev/null || true
git init -q -b main
git commit -q --allow-empty -m "init"

echo "==> [E2E-B] adding marketplace ${MARKETPLACE_REPO}"
claude plugin marketplace add "${MARKETPLACE_REPO}"

echo "==> [E2E-B] installing ${PLUGIN_NAME}@${MARKET_NAME}"
claude plugin install "${PLUGIN_NAME}@${MARKET_NAME}"

# ── pre-bootstrap cache state ───────────────────────────────────────────────
# At this point the plugin source is in the cache but `.real/` should NOT
# exist yet — that's the entire bug PR #14 fixed. We capture the state for
# the post-condition check below.

CACHE_BASE="${HOME}/.claude/plugins/cache/${MARKET_NAME}/${PLUGIN_NAME}"
if [ ! -d "$CACHE_BASE" ]; then
  echo "FAIL: plugin cache not created at $CACHE_BASE — install didn't apply" >&2
  exit 1
fi
VER=$(ls -1 "$CACHE_BASE" | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
CACHE="$CACHE_BASE/$VER"
REAL_BIN="$CACHE/bin/.real/pc-hook-session"

echo "==> [E2E-B] cache layout immediately after install:"
ls -la "$CACHE/bin/" || true
if [ -e "$REAL_BIN" ]; then
  echo "WARN: $REAL_BIN already exists before claude -p — was Phase 2 run somehow?" >&2
  # Not fatal — could happen if the user pre-fetched. But the bootstrap
  # assertion below becomes meaningless.
fi
shim="$CACHE/bin/pc-hook-session"
if [ ! -x "$shim" ]; then
  echo "FAIL: SessionStart shim missing at $shim — wrong cache layout" >&2
  exit 1
fi

echo "==> [E2E-B] firing claude -p (this triggers the SessionStart bootstrap)"
claude -p "$PROMPT"

# ── post-bootstrap cache state ──────────────────────────────────────────────
echo "==> [E2E-B] cache layout after claude -p:"
ls -la "$CACHE/bin/.real/" || true

if [ ! -x "$REAL_BIN" ]; then
  echo "FAIL: $REAL_BIN missing after claude -p — SessionStart bootstrap didn't run" >&2
  echo "      This is the PR #14 regression: marketplace install + bootstrap shim" >&2
  echo "      should self-populate .real/ on first session." >&2
  exit 1
fi
echo "  ✓ SessionStart bootstrap populated .real/pc-hook-session"

echo "==> [E2E-B] asserting capture"
bash /repo/test/e2e/assert-prompt-captured.sh /home/tester/work "$PROMPT"

echo ""
echo "✅ E2E-B passed (marketplace install + bootstrap path)"
