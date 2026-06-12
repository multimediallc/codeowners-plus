#! /usr/bin/env bash
# Decides how the action binary is obtained, emitting mode=download|build
# (plus bin/version/sha256/asset/cache-key) to $GITHUB_OUTPUT. Pins to the
# release tag or a release commit's SHA download the prebuilt binary,
# verified against the checksums baked into the pinned commit (set in
# action.yml only in release commits); all other refs build from the
# checked-out source. A pin never resolves to anything newer than itself.

set -eu

RELEASE_VERSION="${RELEASE_VERSION:-}"

if [ "${RUNNER_OS:-Linux}" != "Linux" ]; then
  echo "Error: codeowners-plus only supports Linux runners (got '${RUNNER_OS}')." >&2
  exit 1
fi
arch=""
expected=""
case "${RUNNER_ARCH:-X64}" in
  X64)
    arch="amd64"
    expected="${SHA256_LINUX_AMD64:-}"
    ;;
  ARM64)
    arch="arm64"
    expected="${SHA256_LINUX_ARM64:-}"
    ;;
esac

{
  echo "bin=${RUNNER_TEMP:-/tmp}/codeowners-plus-action/codeowners-plus"
  echo "version=${RELEASE_VERSION}"
  echo "sha256=${expected}"
} >>"$GITHUB_OUTPUT"

# A release-tag pin or a SHA pin of a release commit downloads the prebuilt
# binary; the embedded checksum makes the result immutable either way.
# Mutable refs (branches) always build from the checked-out source.
pinned_to_release=false
if [ -n "${RELEASE_VERSION}" ] && [ -n "${expected}" ]; then
  if [ "${ACTION_REF:-}" = "${RELEASE_VERSION}" ] || [[ "${ACTION_REF:-}" =~ ^[0-9a-f]{40}$ ]]; then
    pinned_to_release=true
  fi
fi

if [ -n "${ACTION_REPO:-}" ] && [ "${pinned_to_release}" = "true" ] && [ -n "${arch}" ]; then
  echo "Action pinned to release '${RELEASE_VERSION}' (ref '${ACTION_REF}'); using prebuilt binary." >&2
  {
    echo "mode=download"
    echo "asset=codeowners-plus-action_linux_${arch}"
    # Content-addressed: the key names exactly the verified binary bytes.
    echo "cache-key=codeowners-plus-action-bin-${expected}"
  } >>"$GITHUB_OUTPUT"
  exit 0
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
