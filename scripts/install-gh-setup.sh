#!/usr/bin/env bash
set -e

token=${GH_SETUP_GITHUB_TOKEN}
repo="k1LoW/gh-setup"
tag="$(curl -H "Accept: application/vnd.github+json" -H "Authorization: Bearer ${token}" https://api.github.com/repos/${repo}/releases/latest | grep tag_name | awk -F':' '{print $2}' | awk -F'\"' '{print $2}')"
arch="$(uname -m)"

if uname -a | grep Msys > /dev/null; then
  if [ $arch = "x86_64" ]; then
    exe="gh-setup_${tag}_windows_amd64.exe"
  fi
  bin=${TEMP}/gh-setup
elif uname -a | grep Darwin > /dev/null; then
  if [ $arch = "x86_64" ]; then
    exe="gh-setup_${tag}_darwin_amd64"
  elif [ $arch = "arm64" ]; then
    exe="gh-setup_${tag}_darwin_arm64"
  fi
  bin=${TMPDIR}gh-setup
elif uname -a | grep Linux > /dev/null; then
  if [ $arch = "x86_64" ]; then
    exe="gh-setup_${tag}_linux_amd64"
  fi
  bin=/tmp/gh-setup
fi

# download
curl -sL -o ${bin} https://github.com/k1LoW/gh-setup/releases/download/${tag}/${exe}
chmod +x ${bin}
echo "bin=${bin}" >> ${GITHUB_OUTPUT}
