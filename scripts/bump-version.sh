#!/usr/bin/env bash
# Bump the plugin version across all source-of-truth locations:
#   - plugin/.claude-plugin/plugin.json   "version"
#   - cmd/pc-cli/main.go                  Version const
#   - cmd/pc-mcp/main.go                  serverVersion const
#
# Use this on its own to update versions without cutting a release, or via
# scripts/release.sh which calls it.
set -euo pipefail

VER="${1:-}"
if [[ -z "$VER" || ! "$VER" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.-]+)?$ ]]; then
  echo "usage: $0 <semver>   e.g. $0 0.4.0" >&2
  exit 2
fi

REPO=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO"

python3 - "$VER" <<'PY'
import json, re, sys, pathlib
ver = sys.argv[1]

# plugin manifest
p = pathlib.Path("plugin/.claude-plugin/plugin.json")
d = json.loads(p.read_text())
d["version"] = ver
p.write_text(json.dumps(d, indent=2, ensure_ascii=False) + "\n")

# Go const-string rewrites — match `name = "..."` lines.
def patch(path: str, var: str):
    # Match `Version = "..."` whether it follows `const` or sits inside a
    # `const (` block. Word-boundary anchor handles both.
    pattern = re.compile(rf'(\b{re.escape(var)}\s*=\s*)"[^"]*"')
    text = pathlib.Path(path).read_text()
    new, n = pattern.subn(rf'\g<1>"{ver}"', text)
    if n == 0:
        raise SystemExit(f"failed to update {var} in {path} — pattern did not match")
    pathlib.Path(path).write_text(new)

patch("cmd/pc-cli/main.go", "Version")
patch("cmd/pc-mcp/main.go", "serverVersion")
print(f"bumped to v{ver}")
PY

echo "Verify:"
grep -E '"version"|Version =|serverVersion =' \
  plugin/.claude-plugin/plugin.json cmd/pc-cli/main.go cmd/pc-mcp/main.go
