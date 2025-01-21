#!/bin/sh -l

set -e

git config --global --add safe.directory /github/workspace
git branch

/codeowners
