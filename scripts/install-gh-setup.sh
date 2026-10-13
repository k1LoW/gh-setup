#!/usr/bin/env bash
set -euo pipefail

fail() {
  echo "$*" >&2
  exit 1
}

sha256_file() {
  if command -v sha256sum > /dev/null; then
    sha256sum "$1" | cut -d ' ' -f 1
  else
    shasum -a 256 "$1" | cut -d ' ' -f 1
  fi
}

safe_version() {
  local value="$1" upper length
  length="$(printf '%s' "${value}" | wc -c | tr -d ' ')"
  [[ -n "${value}" && "${value}" != "." && "${value}" != ".." && "${length}" -le 128 ]] || return 1
  [[ "${value}" != *[[:cntrl:]]* && "${value}" != *['/\<>:"|?*']* ]] || return 1
  [[ "${value}" != *"." && "${value}" != *" " ]] || return 1
  upper="$(printf '%s' "${value%%.*}" | tr '[:lower:]' '[:upper:]')"
  [[ ! "${upper}" =~ ^(CON|PRN|AUX|NUL|CONIN\$|CONOUT\$|COM[1-9¹²³]|LPT[1-9¹²³])$ ]]
}

cache_version() {
  local tag="$1" version="${1#v}" pre part build
  if [[ "${tag}" =~ ^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$ ]]; then
    build="${version#*+}"
    if [[ "${version}" == *"+"* && ( "${build}" == "."* || "${build}" == *"." || "${build}" == *".."* ) ]]; then
      version="${tag}"
    else
      pre="${version%%+*}"
      if [[ "${pre}" == *"-"* ]]; then
        pre="${pre#*-}"
        if [[ "${pre}" == "."* || "${pre}" == *"." || "${pre}" == *".."* ]]; then
          version="${tag}"
        else
          while [[ -n "${pre}" ]]; do
            part="${pre%%.*}"
            if [[ "${part}" =~ ^0[0-9]+$ ]]; then version="${tag}"; break; fi
            if [[ "${pre}" != *"."* ]]; then break; fi
            pre="${pre#*.}"
          done
        fi
      fi
    fi
  else
    version="${tag}"
  fi
  if safe_version "${version}"; then
    printf '%s' "${version}"
  else
    if command -v sha256sum > /dev/null; then
      printf 'tag-%s' "$(printf '%s' "${version}" | sha256sum | cut -d ' ' -f 1)"
    else
      printf 'tag-%s' "$(printf '%s' "${version}" | shasum -a 256 | cut -d ' ' -f 1)"
    fi
  fi
}

: "${RUNNER_TOOL_CACHE:?}" "${RUNNER_ARCH:?}" "${RUNNER_OS:?}" "${GITHUB_OUTPUT:?}"
[[ "${RUNNER_TOOL_CACHE}" != *$'\n'* && "${RUNNER_TOOL_CACHE}" != *$'\r'* ]] || fail "Invalid RUNNER_TOOL_CACHE"
if [[ "${RUNNER_TOOL_CACHE}" != /* && ! "${RUNNER_TOOL_CACHE}" =~ ^[A-Za-z]:[/\\] ]]; then
  fail "RUNNER_TOOL_CACHE must be an absolute path"
fi

case "${RUNNER_ARCH}" in
  X64) cache_arch="x64"; asset_arch="amd64" ;;
  ARM64) cache_arch="arm64"; asset_arch="arm64" ;;
  X86) cache_arch="ia32"; asset_arch="386" ;;
  ARM) cache_arch="arm"; asset_arch="arm" ;;
  *) fail "Unsupported RUNNER_ARCH: ${RUNNER_ARCH}" ;;
esac
case "${RUNNER_OS}" in
  Linux) asset_os="linux"; executable="gh-setup" ;;
  macOS) asset_os="darwin"; executable="gh-setup" ;;
  Windows) asset_os="windows"; executable="gh-setup.exe" ;;
  *) fail "Unsupported RUNNER_OS: ${RUNNER_OS}" ;;
esac
[[ "${asset_arch}" == "amd64" || ( "${asset_arch}" == "arm64" && "${asset_os}" != "windows" ) ]] ||
  fail "No gh-setup bootstrap asset for ${asset_os}/${asset_arch}"

if ! command -v curl > /dev/null; then
  if command -v apt-get > /dev/null; then
    apt-get update
    apt-get -y install curl ca-certificates
  elif command -v yum > /dev/null; then
    yum install -y curl ca-certificates
  elif command -v apk > /dev/null; then
    apk add --no-cache curl ca-certificates
  else
    fail "curl is required to bootstrap gh-setup"
  fi
fi

tag="${GH_SETUP_GH_SETUP_VERSION:-}"
if [[ -z "${tag}" || "${tag}" == "latest" ]]; then
  token="${GH_SETUP_GITHUB_TOKEN:-${GITHUB_TOKEN:-}}"
  headers=(-H "Accept: application/vnd.github+json")
  if [[ -n "${token}" ]]; then headers+=(-H "Authorization: Bearer ${token}"); fi
  release="$(curl -fsSL "${headers[@]}" https://api.github.com/repos/k1LoW/gh-setup/releases/latest)"
  tag="$(printf '%s\n' "${release}" | sed -n 's/^[[:space:]]*"tag_name":[[:space:]]*"\([^"]*\)".*/\1/p')"
fi
[[ -n "${tag}" ]] || fail "Could not resolve gh-setup release tag"
version="$(cache_version "${tag}")"
tool_root="${RUNNER_TOOL_CACHE}/k1low/gh-setup/${version}/${cache_arch}"
bin="${tool_root}/bin/${executable}"
lock="${tool_root}.lock"
parent="${tool_root%/*}"
current="${RUNNER_TOOL_CACHE}"
for part in k1low gh-setup "${version}" "${cache_arch}"; do
  current="${current}/${part}"
  [[ ! -L "${current}" ]] || fail "Symlink in tool cache path: ${current}"
done
mkdir -p "${parent}"

staging=""
locked=false
directory_moved=false
marker_moved=false
published=false
committed=false
cleanup() {
  local status=$? recovery_failed=false
  trap - EXIT
  if [[ "${committed}" != true ]]; then
    if [[ "${published}" == true ]]; then
      if ! rm -rf "${tool_root}"; then recovery_failed=true; fi
    fi
    if [[ "${directory_moved}" == true ]]; then
      if ! mv "${staging}/previous" "${tool_root}"; then recovery_failed=true; fi
    fi
    if [[ "${marker_moved}" == true ]]; then
      if ! mv "${staging}/previous.complete" "${tool_root}.complete"; then recovery_failed=true; fi
    fi
  fi
  if [[ "${recovery_failed}" == true ]]; then
    echo "Cache rollback failed; recovery files retained at ${staging}" >&2
    status=1
  elif [[ -n "${staging}" ]]; then
    if ! rm -rf "${staging}"; then status=1; fi
  fi
  if [[ "${locked}" == true ]]; then
    if ! rmdir "${lock}"; then status=1; fi
  fi
  exit "${status}"
}
trap cleanup EXIT
deadline=$((SECONDS + 120))
until mkdir "${lock}" 2>/dev/null; do
  [[ -d "${lock}" && ! -L "${lock}" ]] || fail "Cannot acquire tool cache lock: ${lock}"
  (( SECONDS < deadline )) || fail "Timed out waiting for ${lock}; remove abandoned locks manually"
  sleep 1
done
locked=true
current="${RUNNER_TOOL_CACHE}"
for part in k1low gh-setup "${version}" "${cache_arch}"; do
  current="${current}/${part}"
  [[ ! -L "${current}" ]] || fail "Symlink in tool cache path: ${current}"
done

if [[ -f "${tool_root}.complete" && ! -L "${tool_root}.complete" &&
      -x "${bin}" && ! -L "${bin}" && ! -L "${tool_root}/bin" &&
      -f "${tool_root}/bootstrap.tag" && -f "${tool_root}/bootstrap.sha256" ]]; then
  [[ "$(cat "${tool_root}/bootstrap.tag")" == "${tag}" ]] || fail "Bootstrap cache identity collision: ${tool_root}"
  if [[ "$(sha256_file "${bin}")" == "$(cat "${tool_root}/bootstrap.sha256")" ]]; then
    "${bin}" version
    printf 'bin=%s\n' "${bin}" >> "${GITHUB_OUTPUT}"
    exit 0
  fi
  echo "Bootstrap cache integrity check failed; reinstalling ${tag}" >&2
fi

staging="$(mktemp -d "${parent}/.gh-setup-staging-XXXXXX")"
mkdir "${staging}/executables"
asset="gh-setup_${tag}_${asset_os}_${asset_arch}"
if [[ "${asset_os}" == "windows" ]]; then asset="${asset}.exe"; fi
encoded_tag="$(printf '%s' "${tag}" | od -An -tx1 | tr -d ' \n' | sed 's/../%&/g')"
encoded_asset="$(printf '%s' "${asset}" | od -An -tx1 | tr -d ' \n' | sed 's/../%&/g')"
curl -fsSL -o "${staging}/executables/${executable}" \
  "https://github.com/k1LoW/gh-setup/releases/download/${encoded_tag}/${encoded_asset}"
chmod +x "${staging}/executables/${executable}"
"${staging}/executables/${executable}" version
printf '%s\n' "${tag}" > "${staging}/bootstrap.tag"
sha256_file "${staging}/executables/${executable}" > "${staging}/bootstrap.sha256"

if [[ -e "${tool_root}.complete" ]]; then
  mv "${tool_root}.complete" "${staging}/previous.complete"
  marker_moved=true
fi
if [[ -e "${tool_root}" ]]; then
  mv "${tool_root}" "${staging}/previous"
  directory_moved=true
fi
mkdir "${staging}/next"
mv "${staging}/executables" "${staging}/next/bin"
mv "${staging}/bootstrap.tag" "${staging}/bootstrap.sha256" "${staging}/next/"
mv "${staging}/next" "${tool_root}"
published=true
: > "${tool_root}.complete"
committed=true
printf 'bin=%s\n' "${bin}" >> "${GITHUB_OUTPUT}"
