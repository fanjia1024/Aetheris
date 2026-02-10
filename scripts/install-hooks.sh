#!/usr/bin/env bash
# Install Git hooks for this repo (e.g. pre-commit gofmt). Run once after clone.
set -e
root=$(git rev-parse --show-toplevel)
cd "$root"
git config core.hooksPath .githooks
echo "Hooks installed: Git will use .githooks for this repository."
