#!/usr/bin/env bash
set -euo pipefail

bin="${GH_SETUP_BIN:?}"
export GITHUB_TOKEN="${GH_SETUP_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"

args=(
  "--repo=${GH_SETUP_REPO:-}"
  "--version=${GH_SETUP_VERSION:-}"
  "--os=${GH_SETUP_OS:-}"
  "--arch=${GH_SETUP_ARCH:-}"
  "--match=${GH_SETUP_MATCH:-}"
  "--bin-dir=${GH_SETUP_BIN_DIR:-}"
  "--bin-match=${GH_SETUP_BIN_MATCH:-}"
  "--checksum=${GH_SETUP_CHECKSUM:-}"
  "--attestation-flags=${GH_SETUP_ATTESTATION_FLAGS:-}"
)

help="$("${bin}" --help)"
if [[ -z "${GH_SETUP_BIN_DIR:-}" ]]; then
  if ! grep -q -- '--tool-cache' <<< "${help}"; then
    echo "The selected gh-setup-version does not support Runner Tool Cache. Select a release with --tool-cache support, or set bin-dir to opt out." >&2
    exit 1
  fi
  args+=(--tool-cache)
fi

if [[ -n "${GH_SETUP_FORCE:-}" && "${GH_SETUP_FORCE}" != "false" ]]; then
  args+=(--force)
fi
if [[ -n "${GH_SETUP_STRICT:-}" && "${GH_SETUP_STRICT}" != "false" ]]; then
  args+=(--strict)
fi
if [[ -n "${GH_SETUP_SKIP_CONTENT_TYPE_CHECK:-}" && "${GH_SETUP_SKIP_CONTENT_TYPE_CHECK}" != "false" ]]; then
  args+=(--skip-content-type-check)
fi
if [[ -n "${GH_SETUP_VERIFY_ATTESTATION:-}" && "${GH_SETUP_VERIFY_ATTESTATION}" != "false" ]]; then
  args+=(--verify-attestation)
fi

"${bin}" "${args[@]}"

if [[ -n "${GH_SETUP_BIN_DIR:-}" && "${help}" != *"--tool-cache"* ]]; then
  # Older releases do not register explicit installation directories.
  printf '%s\n' "$(cd "${GH_SETUP_BIN_DIR}" && pwd)" >> "${GITHUB_PATH:?}"
fi
