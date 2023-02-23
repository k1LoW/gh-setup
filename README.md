# gh-setup

:octocat: Setup asset of Github Releases.

[![build](https://github.com/k1LoW/gh-setup/actions/workflows/ci.yml/badge.svg)](https://github.com/k1LoW/gh-setup/actions/workflows/ci.yml) ![Coverage](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/gh-setup/coverage.svg) ![Code to Test Ratio](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/gh-setup/ratio.svg)

Key features of `gh-setup` are:

- **For setup, detect the version, the appropriate GitHub Releases asset, the asset's compressed format, and the executable path where the binary will be installed.**
- **Works as a GitHub CLI extension (or a standalone CLI) as well as a GitHub Action.**
- **Could be used as a part to create a GitHub Action like `setup-*`.**

## As a GitHub CLI extension

### Usage

``` console
$ gh setup --repo k1LoW/tbls
Use tbls_v1.62.0_darwin_arm64.zip
Setup binaries to executable path (PATH):
  tbls -> /Users/k1low/local/bin/tbls
$ tbls version
1.62.0
```

### Install

``` console
$ gh extension install k1LoW/gh-setup
```

## As a GitHub Action

### Usage

``` yaml
# .github/workflows/doc.yml
[...]
    steps:
      -
        name: Setup k1LoW/tbls
        uses: k1LoW/gh-setup@v1
        with:
          github-token: ${{ secrets.GITHUB_TOKEN }}
          repo: k1LoW/tbls
        # version: v1.60.0
        # os: linux
        # arch: amd64
        # bin-match: tbls
        # force: true
        # strict: true
        # gh-setup-version: latest
      -
        name: Run tbls
        run: tbls doc
```

## As a part to create a GitHub Action like `setup-*`

See https://github.com/k1LoW/setup-tbls

``` yaml
# action.yml
name: 'Setup tbls'
description: 'GitHub Action for tbls, a CI-Friendly tool for document a database, written in Go.'
branding:
  icon: 'box'
  color: 'blue'
inputs:
  github-token:
    description: The GitHub token
    default: ${{ github.token }}
    required: false
  version:
    description: Version of tbls
    default: latest
    required: false
  force:
    description: Enable force setup
    default: ''
    required: false
runs:
  using: 'composite'
  steps:
    -
      uses: k1LoW/gh-setup@v1
      with:
        repo: github.com/k1LoW/tbls
        github-token: ${{ inputs.github-token }}
        version: ${{ inputs.version }}
        bin-match: tbls
        force: ${{ inputs.force }}
```

## As a Standalone CLI

### Usage

Run `gh-setup` instead of `gh setup`.

``` console
$ gh-setup --repo k1LoW/tbls
Use tbls_v1.62.0_darwin_arm64.zip
Setup binaries to executable path (PATH):
  tbls -> /Users/k1low/local/bin/tbls
$ tbls version
1.62.0
```

### Install

**deb:**

``` console
$ export GH_SETUP_VERSION=X.X.X
$ curl -o gh-setup.deb -L https://github.com/k1LoW/gh-setup/releases/download/v$GH_SETUP_VERSION/gh-setup_$GH_SETUP_VERSION-1_amd64.deb
$ dpkg -i gh-setup.deb
```

**RPM:**

``` console
$ export GH_SETUP_VERSION=X.X.X
$ yum install https://github.com/k1LoW/gh-setup/releases/download/v$GH_SETUP_VERSION/gh-setup_$GH_SETUP_VERSION-1_amd64.rpm
```

**apk:**

``` console
$ export GH_SETUP_VERSION=X.X.X
$ curl -o gh-setup.apk -L https://github.com/k1LoW/gh-setup/releases/download/v$GH_SETUP_VERSION/gh-setup_$GH_SETUP_VERSION-1_amd64.apk
$ apk add gh-setup.apk
```

**homebrew tap:**

```console
$ brew install k1LoW/tap/gh-setup
```

**manually:**

Download binary from [releases page](https://github.com/k1LoW/gh-setup/releases)

**go install:**

```console
$ go install github.com/k1LoW/gh-setup@latest
```

**docker:**

```console
$ docker pull ghcr.io/k1low/gh-setup:latest
```
