#!/usr/bin/env bash
set -e

bin="${GH_SETUP_BIN}"
token=${GH_SETUP_GITHUB_TOKEN}
if [ -z "${token}" ]; then
  token=${GITHUB_TOKEN}
fi
repo=${GH_SETUP_REPO}
bindir=${GH_SETUP_BIN_DIR}
force=${GH_SETUP_FORCE}

${bin} --repo ${repo} --bin-dir=${GH_SETUP_BIN_DIR}
