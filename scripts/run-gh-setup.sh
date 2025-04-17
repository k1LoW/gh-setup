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
checksum=${GH_SETUP_CHECKSUM}
force=${GH_SETUP_FORCE}
strict=${GH_SETUP_STRICT}
skip_content_type_check=${GH_SETUP_SKIP_CONTENT_TYPE_CHECK}
verify_attestation=${GH_SETUP_VERIFY_ATTESTATION}
attestation_flags=${GH_SETUP_ATTESTATION_FLAGS}

boolopts=""

if [ ! -z "${force}" ]; then
    boolopts+=" --force"
fi

if [ ! -z "${strict}" ]; then
    boolopts+=" --strict"
fi

if [ ! -z "${skip_content_type_check}" ]; then
    boolopts+=" --skip-content-type-check"
fi

if [ ! -z "${verify_attestation}" ]; then
    boolopts+=" --verify-attestation"
fi

attestation_flags_opt=""
if [ ! -z "${attestation_flags}" ]; then
    attestation_flags_opt=" --attestation-flags=\"${attestation_flags}\""
fi

${bin} --repo ${repo} --version=${version} --os=${os} --arch=${arch} --match=${match} --bin-dir=${bin_dir} --bin-match=${bin_match} --checksum=${checksum}${boolopts}${attestation_flags_opt}
