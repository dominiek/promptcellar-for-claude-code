#!/usr/bin/env bash
# E2E-A: install Promptcellar via the `curl|sh` route, then prove a real
# `claude -p` round trip lands a captured PLF record in .prompts/.
#
# Runs INSIDE the test container. Requires:
#   ANTHROPIC_API_KEY    (passed in via `docker run -e`)
#   /repo                (bind-mount of the plugin checkout)
#   /home/tester/work    (the throwaway git repo we capture into; created in image)
#
# Default: runs the installer from the local checkout (`bash /repo/install/install.sh`)
# so it matches whatever's on disk. Override with PC_INSTALLER=https://get.promptcellar.io/claude-code
# to test the deployed installer URL too — slower and adds a network dep, but
# proves the pipe-to-shell path serves what we expect.

set -euo pipefail

PROMPT="${EXPECTED_PROMPT:-what time is it}"
INSTALLER="${PC_INSTALLER:-bash /repo/install/install.sh}"

if [ -z "${ANTHROPIC_API_KEY:-}" ]; then
  echo 'ANTHROPIC_API_KEY not set — required for `claude -p` round trip' >&2
  exit 2
fi

cd /home/tester/work

# Fresh git repo each scenario run. Promptcellar refuses to capture in a
# non-git directory by default (see implementation plan), so the `git init`
# + initial commit are load-bearing, not cosmetic.
echo "==> [E2E-A] preparing fresh /work git repo"
rm -rf .prompts .promptcellar .git ./* ./.[!.]* 2>/dev/null || true
git init -q -b main
git commit -q --allow-empty -m "init"

echo "==> [E2E-A] running installer: ${INSTALLER}"
# shellcheck disable=SC2086 # PC_INSTALLER may be a multi-token command
eval ${INSTALLER}

echo "==> [E2E-A] claude plugin list:"
claude plugin list || true

echo "==> [E2E-A] firing claude -p"
# --model not pinned — uses whatever the user's default is. If you want a
# cheap fast model for cost control, set ANTHROPIC_MODEL or pass --model.
claude -p "$PROMPT"

echo "==> [E2E-A] asserting capture"
bash /repo/test/e2e/assert-prompt-captured.sh /home/tester/work "$PROMPT"

echo ""
echo "✅ E2E-A passed (curl|sh install path)"
