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
        # checksum: f1ee97bbf22d5324ec2b468d83f43088d9e5c61deb77fafc220b297e03d47574
        # force: true
        # strict: true
        # verify-attestation: true
        # attestation-flags: "--owner=k1LoW"
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
  checksum:
    description: Checksum of tbls
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
        checksum: ${{ inputs.checksum }}
        force: ${{ inputs.force }}
```

## Runner Tool Cache in GitHub Actions

When `GITHUB_ACTIONS=true`, gh-setup installs into the runner's versioned tool
cache by default, instead of choosing a writable directory already on PATH:

```text
$RUNNER_TOOL_CACHE/<owner>/<repo>/<version>/<cache-arch>/bin/<executable>
$RUNNER_TOOL_CACHE/<owner>/<repo>/<version>/<cache-arch>.complete
```

The tool identity is the lowercased repository owner and name. A valid SemVer
release tag is normalized by removing a leading `v`, while preserving prerelease
and build identifiers. Empty versions and `latest` are resolved to the actual
release tag before cache lookup. Safe non-SemVer tags are kept unchanged; tags
containing separators, unsafe filesystem characters, reserved Windows names, or
excessive length use a deterministic `tag-<SHA-256>` directory instead.
Release lookup always uses the original tag.

The cache architecture comes from `RUNNER_ARCH`: `X86` becomes `ia32`, and `X64`,
`ARM`, and `ARM64` become `x64`, `arm`, and `arm64`. Asset selection uses separate
OS/architecture names, such as `linux/amd64`. Explicit `--os` and `--arch` still
control asset selection, not the runner architecture used in the cache path.

Asset and binary selection options (`--match`, `--bin-match`, `--os`, `--arch`,
`--strict`, and `--skip-content-type-check`) add a deterministic fingerprint to
the repository component. Non-default GitHub hosts also participate in this
fingerprint. Different selections therefore coexist without overwriting each
other or adding a directory level:

```text
$RUNNER_TOOL_CACHE/<owner>/<repo>-<fingerprint>/<version>/<cache-arch>/bin
```

An entry is reusable only when its directory, adjacent `.complete` marker,
gh-setup manifest, and recorded binaries are valid. Binary contents are checked
against recorded SHA-256 digests. The manifest also records the resolved tag,
asset identity, selection, and successful checksum/attestation requirements.
Different or stronger verification requirements trigger a fresh download and
verification rather than trusting an inadequately verified entry.
Entries created by other installers without this metadata are reinstalled.

On a cache hit, gh-setup skips downloading, extracting, and rewriting binaries.
It still resolves release metadata and appends the absolute `bin` directory to
`GITHUB_PATH`, so this is not an offline cache. PATH changes apply to **subsequent
steps**; a CLI invocation cannot change its parent shell's PATH.
`RUNNER_TOOL_CACHE`, `RUNNER_ARCH`, `RUNNER_OS`, and a writable `GITHUB_PATH` are
required for automatic cache mode. Invalid environment settings are errors,
not a fallback to installing elsewhere.

`--force` (Action input `force: true`) bypasses reuse. Downloads and verification
complete before publication, and a failed refresh preserves the previous
completed installation. A per-entry directory lock coordinates concurrent
gh-setup installers; abandoned locks require manual removal. Older versions are
not automatically deleted, and no existing unversioned installations are moved
or removed.

An explicit `--bin-dir` (Action input `bin-dir`) opts out of cache placement and
reuse, preserving the existing overwrite/Force behavior. In Actions, successful
explicit-directory installations also register that directory in `GITHUB_PATH`.
Outside Actions, the standalone CLI and GitHub CLI extension retain their
existing PATH-directory installation behavior. `--tool-cache` explicitly enables
cache mode when the required runner environment is supplied.

The Action's bootstrap gh-setup executable is also cached by version and
architecture, with a completion marker and a recorded digest. The Action invokes
it by absolute path and does not add the bootstrap executable to PATH.

Versioned tool-cache placement supports repeated setup in a job and reuse on
persistent self-hosted runners. It is also suitable for agent sandboxes that
expose Runner Tool Cache; gh-setup does not configure sandbox mounts or control
the order in which a sandbox discovers multiple versions. Disposable hosted
runners should not be expected to retain newly installed tools across jobs.
Non-SemVer cache entries follow the layout convention but are not guaranteed to
be discoverable through `@actions/tool-cache`'s SemVer-specific lookup.

### Action release transition

Automatic tool caching requires a gh-setup executable whose help includes
`--tool-cache`. The Action reports an actionable error for older executables
unless `bin-dir` is explicitly set. The currently pinned bootstrap default
(`v1.11.10`) predates this feature; it must be updated to a release containing
these changes as part of the coordinated release. Before that release, use
`bin-dir` for compatibility. CI builds this checkout into an isolated bootstrap
entry so it tests the new implementation rather than an older published binary.

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

**[aqua](https://aquaproj.github.io/):**

```console
$ aqua g -i k1LoW/gh-setup
```

**manually:**

Download binary from [releases page](https://github.com/k1LoW/gh-setup/releases)

**go install:**

```console
$ go install github.com/k1LoW/gh-setup/cmd/gh-setup@latest
```

**docker:**

```console
$ docker pull ghcr.io/k1low/gh-setup:latest
```

## Attestation Verification

gh-setup supports verifying attestations using the `gh attestation verify` command. To enable attestation verification, use the `--verify-attestation` flag. You can also pass additional flags to the `gh attestation verify` command using the `--attestation-flags` flag.

Example:

```console
$ gh setup --repo k1LoW/tbls --verify-attestation --attestation-flags "--owner=k1LoW"
```

In GitHub Actions:

```yaml
- name: Setup k1LoW/tbls
  uses: k1LoW/gh-setup@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    repo: k1LoW/tbls
    verify-attestation: true
    attestation-flags: "--owner=k1LoW"
```

This feature requires the `gh` CLI with the attestation extension to be installed.
