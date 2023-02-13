# gh-setup

:octocat: Setup asset of Github Releases.

[![build](https://github.com/k1LoW/gh-setup/actions/workflows/ci.yml/badge.svg)](https://github.com/k1LoW/gh-setup/actions/workflows/ci.yml) ![Coverage](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/gh-setup/coverage.svg) ![Code to Test Ratio](https://raw.githubusercontent.com/k1LoW/octocovs/main/badges/k1LoW/gh-setup/ratio.svg)

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
$ gh extension install k1LoW/gh-grep
```

## As a GitHub Action

### Usage

``` yaml
# .github/workflows/doc.yml
[...]
    steps:
      -
        name: Setup k1LoW/tbls
        run: k1LoW/gh-setup@v0
        with:
          github-token: ${secrets.GITHUB_TOKEN}
          repo: k1LoW/tbls
        # version: v1.60.0
        # os: linux
        # arch: amd64
        # force: true
        # gh-setup-version: latest
      -
        name: Run tbls
        run: tbls doc
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
$ docker pull ghcr.io/k1low/gh-grep:latest
```
