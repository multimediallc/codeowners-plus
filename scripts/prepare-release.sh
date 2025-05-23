#! /usr/bin/env bash

set -e
set -u

# --- Configuration ---
ACTIONS_FILE="action.yml"
CLI_TOOL_FILE="tools/cli/main.go"
README_FILE="README.md"

# --- Helper Functions ---
function usage() {
  echo "Usage: $0 <semantic_version>"
  echo "Example: $0 1.2.3"
  echo "  This script will:"
  echo "  1. Create a new branch called 'release/v<semantic_version>'."
  echo "  2. Update '${ACTIONS_FILE}' and ${CLI_TOOL_FILE} so reference the new version."
  echo "  3. Commit the changes."
  echo "  4. Create a tag called 'v<semantic_version>'."
  exit 1
}

function check_git_clean() {
  if ! git diff-index --quiet HEAD --; then
    echo "Error: Your Git working directory is not clean."
    echo "Please commit or stash your changes before running this script."
    exit 1
  fi
  if git rev-parse --verify "refs/tags/${VERSION_TAG}" >/dev/null 2>&1; then
    echo "Error: Tag '${VERSION_TAG}' already exists."
    exit 1
  fi
  git fetch origin
  echo "Switching to origin/main"
  git checkout origin/main >/dev/null 2>&1
  echo "Git working directory is clean."
}

# --- Main Script ---

# Check for argument
if [ -z "${1-}" ]; then
  echo "Error: No semantic version provided."
  usage
fi

SEMANTIC_VERSION="$1"

if ! [[ "$SEMANTIC_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-rc|.dev)?([0-9]+)?$ ]]; then
  echo "Error: '$SEMANTIC_VERSION' does not look like a valid semantic version (e.g., 1.2.3, 1.0.0-alpha.1)."
  usage
fi

VERSION_TAG="v${SEMANTIC_VERSION}"
BRANCH_NAME="release/${VERSION_TAG}"

echo "--- Starting release process for version ${SEMANTIC_VERSION} ---"

check_git_clean

if [ ! -f "${ACTIONS_FILE}" ]; then
  echo "Error: ${ACTIONS_FILE} not found in the current directory.  Make sure you are running this from the root."
  exit 1
fi

echo "Creating branch '${BRANCH_NAME}'..."
if git rev-parse --verify "${BRANCH_NAME}" >/dev/null 2>&1; then
  echo "Error: Branch '${BRANCH_NAME}' already exists."
  git checkout "${BRANCH_NAME}"
else
  git checkout -b "${BRANCH_NAME}"
fi

# Update actions.yml
echo "Updating ${ACTIONS_FILE}, ${CLI_TOOL_FILE}, and ${README_FILE} to replace 'latest' or old tag with '${VERSION_TAG}'..."

# sed -i works differently on macOS and Linux.
# For GNU sed (Linux), -i without an argument is fine.
# For BSD sed (macOS), -i requires an argument (even if empty string for no backup).
if sed --version 2>/dev/null | grep -q GNU; then # GNU sed
  sed -i "s|codeowners-plus:.*'|codeowners-plus:${VERSION_TAG}'|g" "${ACTIONS_FILE}"
  sed -i "s|Version: .*|Version: \"${VERSION_TAG}\",|g" "${CLI_TOOL_FILE}"
  sed -i "s|codeowners-plus@.*|codeowners-plus@${VERSION_TAG}|g" "${README_FILE}"
else # BSD sed (macOS)
  sed -i '' "s|codeowners-plus:.*'|codeowners-plus:${VERSION_TAG}'|g" "${ACTIONS_FILE}"
  sed -i '' "s|Version: .*|Version: \"${VERSION_TAG}\",|g" "${CLI_TOOL_FILE}"
  sed -i '' "s|codeowners-plus@.*|codeowners-plus@${VERSION_TAG}|g" "${README_FILE}"
fi
gofmt -w tools/cli
echo "${ACTIONS_FILE}, ${CLI_TOOL_FILE}, and ${README_FILE} updated."

# Commit the changes
echo "Committing changes to ${ACTIONS_FILE}..."
git add "${ACTIONS_FILE}"
git add "${CLI_TOOL_FILE}"
git commit -m "${VERSION_TAG}"

# Create tag
echo "Creating tag '${VERSION_TAG}'..."
git tag -m "${VERSION_TAG}" "${VERSION_TAG}"

echo "--- Release process completed successfully! ---"
echo ""
echo "Branch '${BRANCH_NAME}' created and switched to."
echo "Tag '${VERSION_TAG}' created locally."
echo ""
echo "Next steps:"
echo "  1. Review the changes: git diff origin/main"
echo "  2. Push the tag: git push origin ${VERSION_TAG}"
echo ""

exit 0
