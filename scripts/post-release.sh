#! /usr/bin/env bash

set -e
set -u

CLI_TOOL_FILE="tools/cli/main.go"
README_FILE="README.md"

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
  echo "Git working directory is clean."
  git fetch origin
  echo "Switching to origin/main"
  git checkout origin/main >/dev/null 2>&1
}

# Check for argument
if [ -z "${1-}" ]; then
  echo "Error: No semantic version provided."
  usage
fi

SEMANTIC_VERSION="$(gh release list --limit 1 --json tagName --jq '.[0].tagName')"

VERSION_TAG="v${SEMANTIC_VERSION}"
DEV_TAG="$(echo "${VERSION_TAG}" | awk -F'[.-]' '{print $1"."$2"."$3+1".dev"}')"
BRANCH_NAME="post/${VERSION_TAG}"

echo "--- Starting update process for version ${SEMANTIC_VERSION} ---"

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

echo "Updating ${CLI_TOOL_FILE} and ${README_FILE}..."

# sed -i works differently on macOS and Linux.
# For GNU sed (Linux), -i without an argument is fine.
# For BSD sed (macOS), -i requires an argument (even if empty string for no backup).
if sed --version 2>/dev/null | grep -q GNU; then # GNU sed
  sed -i "s|Version: .*|Version: \"${DEV_TAG}\",|g" "${CLI_TOOL_FILE}"
  sed -i "s|codeowners-plus@.*|codeowners-plus@${VERSION_TAG}|g" "${README_FILE}"
else # BSD sed (macOS)
  sed -i '' "s|Version: .*|Version: \"${DEV_TAG}\",|g" "${CLI_TOOL_FILE}"
  sed -i '' "s|codeowners-plus@.*|codeowners-plus@${VERSION_TAG}|g" "${README_FILE}"
fi
gofmt -w tools/cli
echo "${CLI_TOOL_FILE} and ${README_FILE} updated."

echo "Committing changes to ${ACTIONS_FILE}..."
git add "${ACTIONS_FILE}" "${CLI_TOOL_FILE}" "${README_FILE}"
git commit -m "${VERSION_TAG}"

echo "--- Post release process completed successfully! ---"
echo ""
echo "Branch '${BRANCH_NAME}' created locally."
echo ""
echo "Next steps:"
echo "  1. Review the changes: git diff origin/main"
echo "  2. Push the branch: git push origin ${BRANCH_NAME}"
echo "  3. Create a pull request on GitHub to merge into main."
echo ""

exit 0
