#! /usr/bin/env bash
# Installs the prebuilt codeowners-cli binary for the current platform,
# verified against the release's checksums.txt.
#
# The companion script install-action.sh does the same for the GitHub Action
# binary. They are kept separate because the two release archives follow
# different goreleaser naming templates.
#
# Local use (all env vars optional):
#   scripts/install-cli.sh                             # latest release -> ./codeowners-cli
#   VERSION=v1.11.0 scripts/install-cli.sh             # a specific release
#   BIN=/usr/local/bin/codeowners-cli scripts/install-cli.sh
#   curl -fsSL https://raw.githubusercontent.com/multimediallc/codeowners-plus/main/scripts/install-cli.sh | bash
#
# Overrides: REPO, VERSION (or TAG), OS, ARCH, BIN. The junit-owners action
# passes REPO/TAG/BIN; OS and ARCH are detected here so the script is
# self-contained.

set -eu

REPO="${REPO:-multimediallc/codeowners-plus}"
BIN="${BIN:-./codeowners-cli}"
TAG="${TAG:-${VERSION:-}}"

# Detect OS unless overridden. The CLI archive title-cases the goreleaser
# {{ .Os }} token, so these are capitalized.
OS="${OS:-}"
if [ -z "${OS}" ]; then
  case "$(uname -s)" in
    Linux)  OS="Linux" ;;
    Darwin) OS="Darwin" ;;
    *)
      echo "Error: unsupported OS '$(uname -s)' (supported: Linux, Darwin)." >&2
      exit 1
      ;;
  esac
fi

# Detect ARCH unless overridden. The CLI archive spells amd64 as x86_64.
ARCH="${ARCH:-}"
if [ -z "${ARCH}" ]; then
  case "$(uname -m)" in
    x86_64 | amd64)  ARCH="x86_64" ;;
    arm64 | aarch64) ARCH="arm64" ;;
    *)
      echo "Error: unsupported arch '$(uname -m)' (supported: x86_64, arm64)." >&2
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

# The CLI archive embeds the version without its leading "v".
asset="codeowners-cli_${TAG#v}_${OS}_${ARCH}.tar.gz"
binname="codeowners-cli"
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

echo "Extracting ${binname} from ${asset}" >&2
tar -xzf "${tmp}/${asset}" -C "${tmp}" "${binname}"

mkdir -p "$(dirname "${BIN}")"
mv "${tmp}/${binname}" "${BIN}"
chmod +x "${BIN}"
echo "Installed ${binname} ${TAG} to ${BIN}" >&2
