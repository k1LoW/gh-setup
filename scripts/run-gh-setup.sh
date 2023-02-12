#!/usr/bin/env bash
set -e

bin="${GH_SETUP_BIN}"
token=${GH_SETUP_GITHUB_TOKEN}
if [ -z "${token}" ]; then
  token=${GITHUB_TOKEN}
fi
repo=${GH_SETUP_REPO}
bindir=${GH_SETUP_BIN_DIR}
version=${GH_SETUP_VERSION}
os=${GH_SETUP_OS}
arch=${GH_SETUP_ARCH}
force=${GH_SETUP_FORCE}

if [ -z "${force}" ]; then
  ${bin} --repo ${repo} --bin-dir=${bindir} --release-version=${version} --os=${os} --arch=${arch}
else
  ${bin} --repo ${repo} --bin-dir=${bindir} --release-version=${version} --os=${os} --arch=${arch} --force
fi
