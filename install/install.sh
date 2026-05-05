#!/usr/bin/env sh
# Promptcellar for Claude Code — production installer.
#
# Run via:   curl -fsSL https://get.promptcellar.io/claude-code | sh
#
# Two phases:
#   1. Add the public marketplace and install the plugin via Claude Code's own
#      `claude plugin install` flow — the only path that produces a working
#      install (direct cache-dir + installed_plugins.json edits result in
#      disabled-status plugins).
#   2. Fetch the matching platform tarball from the GitHub release and extract
#      its `bin/` into the plugin cache. The marketplace flow only ships the
#      source tree (binaries are gitignored), so this step is mandatory for
#      hooks to fire.

set -eu

MARKETPLACE_REPO="${PC_MARKETPLACE:-dominiek/promptcellar-for-claude-code}"
MARKET_NAME="${PC_MARKET_NAME:-promptcellar}"
PLUGIN_NAME="${PC_PLUGIN_NAME:-promptcellar}"
RELEASES_URL="${PC_RELEASES_URL:-https://github.com/dominiek/promptcellar-for-claude-code/releases/download}"

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

# ── Phase 2: fetch + install platform binaries ──────────────────────────────

CACHE_BASE="${HOME}/.claude/plugins/cache/${MARKET_NAME}/${PLUGIN_NAME}"
if [ ! -d "${CACHE_BASE}" ]; then
  echo "==> plugin cache not found at ${CACHE_BASE} — was the install actually applied?" >&2
  exit 1
fi

# Pick the highest-versioned subdir (sort -V is GNU; on BSD `sort` lacks -V,
# but a directory listing is short so a portable fallback is fine).
VER=$(ls -1 "${CACHE_BASE}" 2>/dev/null | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)
if [ -z "${VER}" ]; then
  echo "==> could not determine installed version under ${CACHE_BASE}" >&2
  exit 1
fi
CACHE="${CACHE_BASE}/${VER}"
echo "==> binary phase: ${PLUGIN_NAME} ${VER} → ${CACHE}/bin"

# Detect platform.
OS=$(uname -s 2>/dev/null || echo unknown)
ARCH=$(uname -m 2>/dev/null || echo unknown)
case "${OS}-${ARCH}" in
  Darwin-arm64)             PLATFORM=darwin-arm64 ;;
  Darwin-x86_64)            PLATFORM=darwin-x64 ;;
  Linux-aarch64|Linux-arm64) PLATFORM=linux-arm64 ;;
  Linux-x86_64|Linux-amd64)  PLATFORM=linux-x64 ;;
  *)
    echo "==> unsupported platform: ${OS}-${ARCH}" >&2
    echo "    supported: darwin-arm64, darwin-x64, linux-arm64, linux-x64" >&2
    exit 1 ;;
esac

ASSET="${PLUGIN_NAME}-${VER}-${PLATFORM}.tar.gz"
TARBALL_URL="${RELEASES_URL}/v${VER}/${ASSET}"
SHA_URL="${TARBALL_URL}.sha256"

TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

echo "==> downloading ${ASSET}"
if ! curl -fsSL -o "${TMPDIR}/${ASSET}" "${TARBALL_URL}"; then
  echo "==> failed to download ${TARBALL_URL}" >&2
  echo "    a release for v${VER} may not exist yet, or the version dir in" >&2
  echo "    the cache is out of sync with the published releases." >&2
  exit 1
fi

if curl -fsSL -o "${TMPDIR}/${ASSET}.sha256" "${SHA_URL}"; then
  echo "==> verifying sha256"
  ( cd "${TMPDIR}" && \
    if command -v shasum >/dev/null 2>&1; then
      shasum -a 256 -c "${ASSET}.sha256" >/dev/null
    elif command -v sha256sum >/dev/null 2>&1; then
      sha256sum -c "${ASSET}.sha256" >/dev/null
    else
      echo "    no shasum/sha256sum available; skipping verification" >&2
    fi )
else
  echo "==> sha256 file not available; skipping verification" >&2
fi

echo "==> extracting bin/ → ${CACHE}/bin"
mkdir -p "${CACHE}/bin"
# Tarball layout (from .github/workflows/release.yml): contents of plugin/ at
# the tar root, including ./bin/. Extract just the bin/ subtree.
tar -xzf "${TMPDIR}/${ASSET}" -C "${CACHE}" "./bin"

# Sanity-check: the six expected binaries should now be in place.
MISSING=""
for b in pc-cli pc-hook-prompt pc-hook-session pc-hook-stop pc-hook-tool pc-mcp; do
  if [ ! -x "${CACHE}/bin/${b}" ]; then
    MISSING="${MISSING} ${b}"
  fi
done
if [ -n "${MISSING}" ]; then
  echo "==> WARNING: missing binaries after extract:${MISSING}" >&2
fi

cat <<EOF

✔ Installed.

Open a NEW Claude Code session in any git repo:
  cd /path/to/your/repo && claude

Inside CC:
  /promptcellar:status      # confirm capture is on
  /promptcellar:log         # see captured prompts
  /promptcellar:disable     # opt out for this repo

Cross-repo workspace (running claude above multiple repos):
  /promptcellar:destination ~/path/to/prompts-store

To uninstall: curl -fsSL https://get.promptcellar.io/claude-code/uninstall | sh
EOF
