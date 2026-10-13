#!/usr/bin/env bash
set -euo pipefail

case "${RUNNER_ARCH:?}" in
  X86) cache_arch="ia32" ;;
  *) cache_arch="$(printf '%s' "${RUNNER_ARCH}" | tr '[:upper:]' '[:lower:]')" ;;
esac
executable="gh-setup"
if [[ "${RUNNER_OS:?}" == "Windows" ]]; then executable="gh-setup.exe"; fi

tool_root="${RUNNER_TOOL_CACHE:?}/k1low/gh-setup/source-test/${cache_arch}"
mkdir -p "${tool_root}/bin"
go build -o "${tool_root}/bin/${executable}" ./cmd/gh-setup
printf 'source-test\n' > "${tool_root}/bootstrap.tag"
if command -v sha256sum > /dev/null; then
  sha256sum "${tool_root}/bin/${executable}" | cut -d ' ' -f 1 > "${tool_root}/bootstrap.sha256"
else
  shasum -a 256 "${tool_root}/bin/${executable}" | cut -d ' ' -f 1 > "${tool_root}/bootstrap.sha256"
fi
: > "${tool_root}.complete"
