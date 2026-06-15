#! /usr/bin/env bash
# Installs the prebuilt codeowners-plus binary for the current platform,
# verified against the release's checksums.txt.
#
# Local use (all env vars optional):
#   scripts/install-action.sh                          # latest release -> ./codeowners-plus
#   VERSION=v1.9.1 scripts/install-action.sh           # a specific release
#   BIN=/usr/local/bin/codeowners-plus scripts/install-action.sh
#   curl -fsSL https://raw.githubusercontent.com/multimediallc/codeowners-plus/main/scripts/install-action.sh | bash
#
# Overrides: REPO, VERSION (or TAG), OS, ARCH, BIN. action.yml passes
# REPO/TAG/BIN; OS and ARCH are detected here so the script is self-contained.

set -eu

REPO="${REPO:-multimediallc/codeowners-plus}"
BIN="${BIN:-./codeowners-plus}"
TAG="${TAG:-${VERSION:-}}"

# Detect OS unless overridden. Tokens match goreleaser's {{ .Os }}.
OS="${OS:-}"
if [ -z "${OS}" ]; then
  case "$(uname -s)" in
    Linux)  OS="linux" ;;
    Darwin) OS="darwin" ;;
    *)
      echo "Error: unsupported OS '$(uname -s)' (supported: linux, darwin)." >&2
      exit 1
      ;;
  esac
fi

# Detect ARCH unless overridden. Tokens match goreleaser's {{ .Arch }}.
ARCH="${ARCH:-}"
if [ -z "${ARCH}" ]; then
  case "$(uname -m)" in
    x86_64 | amd64)  ARCH="amd64" ;;
    arm64 | aarch64) ARCH="arm64" ;;
    *)
      echo "Error: unsupported arch '$(uname -m)' (supported: amd64, arm64)." >&2
      exit 1
      ;;
  esac
fi

# Default to the latest release when no version was requested.
if [ -z "${TAG}" ]; then
  TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | awk -F'"' '/"tag_name":/ {print $4; exit}')"
  if [ -z "${TAG}" ]; then
    echo "Error: could not determine the latest release of ${REPO}." >&2
    exit 1
  fi
fi

asset="codeowners-plus-action_${OS}_${ARCH}"
base="https://github.com/${REPO}/releases/download/${TAG}"
tmp="$(mktemp -d)"
trap 'rm -rf "${tmp}"' EXIT

echo "Downloading ${asset} from ${REPO} release ${TAG}" >&2
curl -fsSL --retry 3 -o "${tmp}/${asset}" "${base}/${asset}"
curl -fsSL --retry 3 -o "${tmp}/checksums.txt" "${base}/checksums.txt"

echo "Verifying ${asset} against checksums.txt" >&2
expected="$(awk -v a="${asset}" '$2 == a {print $1}' "${tmp}/checksums.txt")"
if [ -z "${expected}" ]; then
  echo "Error: ${asset} not found in checksums.txt" >&2
  exit 1
fi
# Guard against a malformed digest: '<checker> -c' treats an improperly
# formatted line as a skipped (passing) entry rather than a failure.
if ! printf '%s' "${expected}" | grep -Eq '^[0-9a-f]{64}$'; then
  echo "Error: invalid checksum for ${asset} in checksums.txt" >&2
  exit 1
fi
# sha256sum is GNU coreutils (Linux); macOS only ships shasum.
if command -v sha256sum >/dev/null 2>&1; then
  verify=(sha256sum -c -)
else
  verify=(shasum -a 256 -c -)
fi
if ! echo "${expected}  ${tmp}/${asset}" | "${verify[@]}"; then
  echo "Error: downloaded ${asset} does not match its release checksum" >&2
  exit 1
fi

mkdir -p "$(dirname "${BIN}")"
mv "${tmp}/${asset}" "${BIN}"
chmod +x "${BIN}"
echo "Installed codeowners-plus ${TAG} to ${BIN}" >&2
