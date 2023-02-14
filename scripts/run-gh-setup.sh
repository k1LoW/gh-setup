#!/usr/bin/env bash
set -e

bin="${GH_SETUP_BIN}"
token=${GH_SETUP_GITHUB_TOKEN}
if [ -z "${token}" ]; then
  token=${GITHUB_TOKEN}
fi
export GITHUB_TOKEN=${token}

repo=${GH_SETUP_REPO}
version=${GH_SETUP_VERSION}
os=${GH_SETUP_OS}
arch=${GH_SETUP_ARCH}
match=${GH_SETUP_MATCH}
bin_dir=${GH_SETUP_BIN_DIR}
bin_match=${GH_SETUP_BIN_MATCH}
force=${GH_SETUP_FORCE}

if [ -z "${force}" ]; then
  ${bin} --repo ${repo} --version=${version} --os=${os} --arch=${arch} --bin-dir=${bin_dir} --bin-match=${bin_match}
else
  ${bin} --repo ${repo} --version=${version} --os=${os} --arch=${arch} --bin-dir=${bin_dir} --bin-match=${bin_match} --force
fi
