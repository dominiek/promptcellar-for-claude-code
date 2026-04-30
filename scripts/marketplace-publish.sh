#!/usr/bin/env bash
# Print a marketplace entry for anthropics/claude-plugins-official, plus the
# PR instructions to land it. This is a SEPARATE step from cutting a release
# tag — running it does not by itself update the public marketplace; it just
# generates the JSON snippet a human submits via PR.
#
# Once the snippet has been merged into anthropics/claude-plugins-official,
# subsequent releases do NOT require resubmitting: the entry below uses
# git-subdir with `ref: main`, so users get the latest tagged release whenever
# they update.
#
# (If you'd rather pin the marketplace entry to a specific tag, swap "main"
# in the `ref` field for "v<X.Y.Z>" — and remember to bump it on every release.)
set -euo pipefail

REPO=$(cd "$(dirname "$0")/.." && pwd)
VER=$(python3 -c "import json; print(json.load(open('$REPO/plugin/.claude-plugin/plugin.json'))['version'])")
SHA=$(git -C "$REPO" rev-parse HEAD)

cat <<EOF
Marketplace publication step
============================

Open a PR against:
  https://github.com/anthropics/claude-plugins-official

Edit:
  .claude-plugin/marketplace.json

Add this object to the "plugins" array (alphabetically by name is the convention):

{
  "name": "promptcellar",
  "description": "Capture every prompt + outcome (files touched, commits, tokens, cost) to .prompts/ in PLF-1 format. Slash commands and an MCP server expose captured history to the agent.",
  "author": { "name": "Promptcellar" },
  "category": "logging",
  "source": {
    "source": "git-subdir",
    "url": "https://github.com/dominiek/promptcellar-for-claude-code.git",
    "path": "plugin",
    "ref": "main",
    "sha": "$SHA"
  },
  "homepage": "https://github.com/dominiek/promptcellar-for-claude-code"
}

Current version of plugin/.claude-plugin/plugin.json:  v$VER

After the PR merges, anyone running Claude Code can install with:
  claude plugin install promptcellar

EOF
