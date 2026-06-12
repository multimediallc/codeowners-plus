#! /usr/bin/env bash
# Installs the release binary at BIN, verified against EXPECTED_SHA256 (the
# checksum from the pinned commit): downloads on a cache miss, re-verifies
# the (untrusted, repo-scoped) cached copy on a cache hit.

set -eu

: "${RELEASE_VERSION:?RELEASE_VERSION must be set}" "${ACTION_REPO:?ACTION_REPO must be set}" \
  "${ASSET:?ASSET must be set}" "${BIN:?BIN must be set}" "${EXPECTED_SHA256:?EXPECTED_SHA256 must be set}"

# Downloads the release binary and installs it to BIN if it matches
# EXPECTED_SHA256.
fetch() {
  local dest
  dest="$(mktemp)"
  echo "Downloading ${ASSET} from release ${RELEASE_VERSION}" >&2
  curl -fsSL --retry 3 -o "${dest}" \
    "https://github.com/${ACTION_REPO}/releases/download/${RELEASE_VERSION}/${ASSET}"
  if ! echo "${EXPECTED_SHA256}  ${dest}" | sha256sum -c --quiet -; then
    echo "Error: downloaded ${ASSET} does not match the checksum in the pinned commit" >&2
    exit 1
  fi
  mkdir -p "$(dirname "${BIN}")"
  mv "${dest}" "${BIN}"
  chmod +x "${BIN}"
}

if [ -x "${BIN}" ]; then
  if echo "${EXPECTED_SHA256}  ${BIN}" | sha256sum -c --quiet - >/dev/null 2>&1; then
    echo "Cached binary checksum matched (${EXPECTED_SHA256})" >&2
    exit 0
  fi
  echo "Warning: cached binary checksum mismatch; discarding cache and re-fetching." >&2
  rm -f "${BIN}"
fi
fetch
