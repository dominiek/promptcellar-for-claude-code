#!/usr/bin/env sh
# Promptcellar — bootstrap helper sourced by the SessionStart shim.
#
# Background: the marketplace install (`claude plugin install`) only ships the
# plugin source tree, so the compiled Go binaries that the hook commands point
# at are missing on first run. This helper downloads the platform-specific
# release tarball from GitHub Releases and extracts the binaries into the
# sibling .real/ directory. After it succeeds, the shim execs the real binary
# and subsequent hook fires find it already in place.
#
# This mirrors Phase 2 of install/install.sh — the only difference is that it
# runs from inside the plugin instead of from a separate `curl | sh` script,
# so a bare `claude plugin install promptcellar@promptcellar` is now
# self-sufficient.

# Usage: pc_bootstrap <shim-dir>
#   <shim-dir> — the directory containing the shim that called us. Real
#                binaries land in <shim-dir>/.real/.
#
# Returns 0 on success (binaries are now in place), non-zero on any failure
# (network down, unsupported platform, manifest missing, sha mismatch, …).
# Callers should treat failure as "skip capture this session" — never block
# the user.
pc_bootstrap() {
  pc__shim_dir=$1
  if [ -z "$pc__shim_dir" ] || [ ! -d "$pc__shim_dir" ]; then
    return 1
  fi

  pc__real_dir="$pc__shim_dir/.real"
  # Already bootstrapped? Nothing to do.
  if [ -x "$pc__real_dir/pc-hook-session" ]; then
    return 0
  fi

  # Need curl + tar to proceed.
  if ! command -v curl >/dev/null 2>&1; then
    return 1
  fi
  if ! command -v tar >/dev/null 2>&1; then
    return 1
  fi

  # Detect platform — same matrix as install/install.sh and the release matrix
  # in .github/workflows/release.yml. Windows is intentionally omitted: the
  # shim itself is POSIX shell, so a Windows install can't reach this path.
  pc__os=$(uname -s 2>/dev/null || echo unknown)
  pc__arch=$(uname -m 2>/dev/null || echo unknown)
  case "${pc__os}-${pc__arch}" in
    Darwin-arm64)               pc__platform=darwin-arm64 ;;
    Darwin-x86_64)              pc__platform=darwin-x64 ;;
    Linux-aarch64|Linux-arm64)  pc__platform=linux-arm64 ;;
    Linux-x86_64|Linux-amd64)   pc__platform=linux-x64 ;;
    *) return 1 ;;
  esac

  # Read the version from the plugin manifest. We deliberately don't shell out
  # to jq — the plugin must work with whatever the user has on PATH.
  pc__manifest="$pc__shim_dir/../.claude-plugin/plugin.json"
  if [ ! -f "$pc__manifest" ]; then
    return 1
  fi
  pc__version=$(sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$pc__manifest" | head -n 1)
  if [ -z "$pc__version" ]; then
    return 1
  fi

  pc__asset="promptcellar-${pc__version}-${pc__platform}.tar.gz"
  pc__url="https://github.com/dominiek/promptcellar-for-claude-code/releases/download/v${pc__version}/${pc__asset}"

  # Stage everything in a tmpdir so a partial download can never leave a
  # half-extracted .real/ behind.
  pc__tmp=$(mktemp -d 2>/dev/null) || pc__tmp="${TMPDIR:-/tmp}/pc-bootstrap-$$"
  mkdir -p "$pc__tmp" || return 1

  # SessionStart's hook timeout is 10s; cap curl well below that so we leave
  # headroom for sha verification and extraction. Users on slow connections
  # will hit this and fall through to the silent-skip path; the next session
  # tries again.
  if ! curl -fsSL --max-time 6 -o "$pc__tmp/$pc__asset" "$pc__url" 2>/dev/null; then
    rm -rf "$pc__tmp"
    return 1
  fi

  # Optional sha256 verification — best-effort, skip if either the .sha256
  # file or a checksum tool isn't available.
  if curl -fsSL --max-time 2 -o "$pc__tmp/$pc__asset.sha256" "${pc__url}.sha256" 2>/dev/null; then
    if command -v shasum >/dev/null 2>&1; then
      if ! ( cd "$pc__tmp" && shasum -a 256 -c "$pc__asset.sha256" >/dev/null 2>&1 ); then
        rm -rf "$pc__tmp"
        return 1
      fi
    elif command -v sha256sum >/dev/null 2>&1; then
      if ! ( cd "$pc__tmp" && sha256sum -c "$pc__asset.sha256" >/dev/null 2>&1 ); then
        rm -rf "$pc__tmp"
        return 1
      fi
    fi
  fi

  pc__stage="$pc__tmp/stage"
  mkdir -p "$pc__stage" || { rm -rf "$pc__tmp"; return 1; }
  if ! tar -xzf "$pc__tmp/$pc__asset" -C "$pc__stage" 2>/dev/null; then
    rm -rf "$pc__tmp"
    return 1
  fi

  # Tarball ships ./bin/.real/<binaries>. (See .github/workflows/release.yml.)
  if [ ! -d "$pc__stage/bin/.real" ]; then
    rm -rf "$pc__tmp"
    return 1
  fi

  mkdir -p "$pc__real_dir" || { rm -rf "$pc__tmp"; return 1; }
  for pc__src in "$pc__stage/bin/.real/"*; do
    [ -f "$pc__src" ] || continue
    pc__name=$(basename "$pc__src")
    chmod +x "$pc__src" 2>/dev/null || true
    # Atomic per-binary swap: write to a temp name in the destination, then
    # mv into place. mv within the same filesystem is atomic.
    pc__dst_tmp="$pc__real_dir/.$pc__name.tmp.$$"
    cp "$pc__src" "$pc__dst_tmp" || { rm -rf "$pc__tmp"; return 1; }
    chmod +x "$pc__dst_tmp" 2>/dev/null || true
    mv "$pc__dst_tmp" "$pc__real_dir/$pc__name" || { rm -rf "$pc__tmp"; return 1; }
  done

  rm -rf "$pc__tmp"
  return 0
}
