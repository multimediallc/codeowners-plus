#! /usr/bin/env bash
# Resolves, fetches, and verifies the codeowners-plus action binary.
# Usage: action-binary.sh resolve|ensure
#
# resolve: Decides how the binary is obtained. RELEASE_VERSION is set in
#   action.yml only in release commits (by prepare-release.sh, cleared by
#   post-release.sh). A pin to the release tag downloads that release's
#   prebuilt binary; any other ref (including a release commit's SHA, since
#   release assets are mutable while a SHA pin promises immutability) builds
#   from the checked-out action source -- a pin never resolves to anything
#   newer than itself.
#   Outputs (via $GITHUB_OUTPUT): mode=download|build, bin, version,
#   cache-key (empty for mutable refs like branches), asset (download only).
#
# ensure: Makes the checksum-verified release binary available at BIN:
#   downloads it on a cache miss, re-verifies the restored copy on a cache
#   hit. See verify() for the cache trust model.

set -e
set -u

RELEASE_VERSION="${RELEASE_VERSION:-}"

# Downloads checksums.txt from the release and prints the expected hash for
# $ASSET. Fails if the asset has no entry.
expected_checksum() {
  local checksums
  checksums="$(mktemp)"
  curl -fsSL --retry 3 -o "${checksums}" \
    "https://github.com/${ACTION_REPO}/releases/download/${RELEASE_VERSION}/checksums.txt"
  grep -E "  ${ASSET}\$" "${checksums}" | awk '{print $1}'
  rm -f "${checksums}"
}

resolve() {
  if [ "${RUNNER_OS:-Linux}" != "Linux" ]; then
    echo "Error: codeowners-plus only supports Linux runners (got '${RUNNER_OS}')." >&2
    exit 1
  fi
  local arch=""
  case "${RUNNER_ARCH:-X64}" in
    X64) arch="amd64" ;;
    ARM64) arch="arm64" ;;
  esac

  {
    echo "bin=${RUNNER_TEMP:-/tmp}/codeowners-plus-action/codeowners-plus"
    echo "version=${RELEASE_VERSION}"
  } >>"$GITHUB_OUTPUT"

  # Only an exact release-tag pin downloads the prebuilt binary. SHA pins
  # always build from the pinned source: release assets are mutable (tags can
  # be re-pointed, assets re-uploaded), so serving them to a SHA pin would
  # silently weaken its immutability guarantee.
  if [ -n "${ACTION_REPO:-}" ] && [ -n "${RELEASE_VERSION}" ] && [ "${ACTION_REF:-}" = "${RELEASE_VERSION}" ] && [ -n "${arch}" ]; then
    echo "Action pinned to release '${RELEASE_VERSION}' (ref '${ACTION_REF}'); using prebuilt binary." >&2
    {
      echo "mode=download"
      echo "asset=codeowners-plus-action_linux_${arch}"
      echo "cache-key=codeowners-plus-action-${RELEASE_VERSION}-linux-${arch}"
    } >>"$GITHUB_OUTPUT"
    return
  fi

  echo "Action ref '${ACTION_REF:-<unknown>}' is not a release; building from source." >&2
  echo "mode=build" >>"$GITHUB_OUTPUT"
  if [[ "${ACTION_REF:-}" =~ ^[0-9a-f]{40}$ ]]; then
    # Immutable commit pin: safe to cache the built binary.
    echo "cache-key=codeowners-plus-action-src-${ACTION_REF}-linux-${arch:-${RUNNER_ARCH:-unknown}}" >>"$GITHUB_OUTPUT"
  else
    # Branch or other mutable ref: never cache, always build fresh.
    echo "cache-key=" >>"$GITHUB_OUTPUT"
  fi
}

# Downloads the release binary, verifies it against the release
# checksums.txt, and installs it to BIN.
fetch() {
  local dest expected
  dest="$(mktemp)"
  echo "Downloading ${ASSET} from release ${RELEASE_VERSION}" >&2
  curl -fsSL --retry 3 -o "${dest}" \
    "https://github.com/${ACTION_REPO}/releases/download/${RELEASE_VERSION}/${ASSET}"
  expected="$(expected_checksum)"
  if [ -z "${expected}" ]; then
    echo "Error: no checksum entry for ${ASSET} in release checksums.txt" >&2
    exit 1
  fi
  if ! echo "${expected}  ${dest}" | sha256sum -c --quiet -; then
    echo "Error: checksum mismatch for downloaded ${ASSET}" >&2
    exit 1
  fi
  mkdir -p "$(dirname "${BIN}")"
  mv "${dest}" "${BIN}"
  chmod +x "${BIN}"
}

# Re-verifies a cache-restored binary. Actions caches are repo-scoped and
# writable by other workflows, so a cached binary is not trusted blindly:
# checksums.txt is re-fetched from the release (the trust anchor) and a
# mismatch discards and re-downloads the binary. If checksums.txt is
# unreachable (GitHub outage), the cached copy is used as-is -- it was
# verified when first cached, and the cache exists to ride out GitHub flake.
verify() {
  local expected
  if ! expected="$(expected_checksum)" || [ -z "${expected}" ]; then
    echo "Warning: could not re-verify cached binary against release checksums.txt; using cached binary (verified when first cached)." >&2
    return
  fi
  if echo "${expected}  ${BIN}" | sha256sum -c --quiet - >/dev/null 2>&1; then
    echo "Cached binary checksum OK (${expected})" >&2
    return
  fi
  echo "Warning: cached binary checksum mismatch; discarding cache and re-fetching." >&2
  rm -f "${BIN}"
  fetch
}

case "${1:-}" in
  resolve) resolve ;;
  ensure)
    : "${RELEASE_VERSION:?RELEASE_VERSION must be set}" "${ACTION_REPO:?ACTION_REPO must be set}" "${ASSET:?ASSET must be set}" "${BIN:?BIN must be set}"
    if [ -f "${BIN}" ]; then verify; else fetch; fi
    ;;
  *)
    echo "Usage: $0 resolve|ensure" >&2
    exit 1
    ;;
esac
