#! /usr/bin/env bash
# Release gate: fails unless the built binary matches the checksum that
# prepare-release.sh embedded in action.yml.
# Usage: verify-release-binary.sh <binary-path> <arch>

set -eu

path="$1"
arch="$2"

key="SHA256_LINUX_$(echo "${arch}" | tr '[:lower:]' '[:upper:]')"
expected="$(grep -E "^ *${key}: '" action.yml | sed -E "s/.*'([^']*)'.*/\1/")"
if [ -z "${expected}" ]; then
  echo "Error: ${key} is not set in action.yml; releases must be cut from a prepare-release.sh commit." >&2
  exit 1
fi

# sha256sum is GNU coreutils; older macOS only ships shasum.
command -v sha256sum >/dev/null && sha256=(sha256sum) || sha256=(shasum -a 256)
echo "${expected}  ${path}" | "${sha256[@]}" -c -
