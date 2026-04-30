#!/usr/bin/env bash
# Cut a release: bump versions, run all tests, commit, tag, push.
#
# After this, GitHub Actions (release.yml) takes over: it cross-compiles the
# six binaries for darwin/linux/windows × arm64/x64, packages tarballs, and
# uploads them to a GitHub Release at the matching tag.
#
# Updating the public Anthropic marketplace listing is a SEPARATE step, run
# only when you want users to be able to install without first adding this
# repo as a marketplace. See scripts/marketplace-publish.sh.
set -euo pipefail

VER="${1:-}"
if [[ -z "$VER" ]]; then
  echo "usage: $0 <semver>   e.g. $0 0.4.0" >&2
  exit 2
fi

REPO=$(cd "$(dirname "$0")/.." && pwd)
cd "$REPO"

if [[ -n "$(git status --porcelain)" ]]; then
  echo "Working tree is dirty. Commit or stash before releasing." >&2
  git status --short
  exit 1
fi

BRANCH=$(git symbolic-ref --short HEAD)
if [[ "$BRANCH" != "main" ]]; then
  echo "Releases are cut from main. You're on '$BRANCH'." >&2
  exit 1
fi

if git rev-parse --verify "v$VER" >/dev/null 2>&1; then
  echo "Tag v$VER already exists." >&2
  exit 1
fi

echo "==> bumping to v$VER"
bash scripts/bump-version.sh "$VER"

echo "==> running full test suite"
make test-all

echo "==> committing version bump"
git add plugin/.claude-plugin/plugin.json cmd/pc-cli/main.go cmd/pc-mcp/main.go
git commit -m "chore: release v$VER"

echo "==> tagging v$VER"
git tag -a "v$VER" -m "v$VER"

echo
echo "About to push:"
echo "  - main (with chore: release v$VER commit)"
echo "  - tag v$VER  (triggers release.yml — cross-compile + GitHub Release upload)"
echo
read -r -p "Push to origin? [y/N] " confirm
case "$confirm" in
  y|Y) ;;
  *) echo "Aborted. Run 'git push origin main && git push origin v$VER' manually when ready." ; exit 1 ;;
esac

git push origin main
git push origin "v$VER"

cat <<EOF

✔ Release v$VER pushed.

Watch CI:
  https://github.com/dominiek/promptcellar-for-claude-code/actions

Once release.yml is green, the tarballs land at:
  https://github.com/dominiek/promptcellar-for-claude-code/releases/tag/v$VER

Optional next step (only if you want this version on the public Anthropic
marketplace, so other users can run \`claude plugin install promptcellar\`
without first adding this repo as a marketplace):

  bash scripts/marketplace-publish.sh
EOF
