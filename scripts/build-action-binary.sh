#! /usr/bin/env bash

# Build script for the github action binary.
# Single source of truth for build settings since we build in both
# prepare-release (to get the binary shasum) and
# in the actual release step (to get the artifact).

set -eu

out="$1"
arch="${2:-$(go env GOARCH)}"

cd "$(dirname "${BASH_SOURCE[0]}")/.."
GOOS=linux GOARCH="${arch}" CGO_ENABLED=0 \
  go build -trimpath -buildvcs=false -ldflags="-s -w" -o "${out}" .
