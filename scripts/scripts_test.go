package scripts

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/k1LoW/gh-setup/setup"
)

func scriptFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func scriptEnvironment(t *testing.T) (string, []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell fixtures use POSIX paths; Windows Action behavior is covered by runner CI")
	}
	dir := t.TempDir()
	fixtures := filepath.Join(dir, "fixtures")
	if err := os.Mkdir(fixtures, 0755); err != nil {
		t.Fatal(err)
	}
	binary := scriptFile(t, fixtures, "fake-gh-setup", `#!/usr/bin/env bash
set -euo pipefail
case "${1:-}" in
  version)
    if [[ "${FAIL_BINARY:-}" == "true" ]]; then exit 1; fi
    echo "gh-setup version fixture"
    ;;
  --help)
    if [[ "${FAKE_CAPABLE:-true}" == "true" ]]; then echo "--tool-cache"; fi
    ;;
  *)
    printf '%s\n' "$@" > "${ARGUMENT_LOG:?}"
    ;;
esac
`)
	scriptFile(t, fixtures, "curl", `#!/usr/bin/env bash
set -euo pipefail
out=""
url=""
while (( $# > 0 )); do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    -H) shift 2 ;;
    -*) shift ;;
    *) url="$1"; shift ;;
  esac
done
printf '%s\n' "${url}" >> "${CURL_LOG:?}"
if [[ "${url}" == *"/releases/latest" ]]; then
  printf '{\n  "tag_name": "v1.2.3"\n}\n'
elif [[ "${FAIL_DOWNLOAD:-}" == "true" ]]; then
  exit 22
else
  cp "${FAKE_BINARY:?}" "${out:?}"
fi
`)
	output := scriptFile(t, dir, "github-output", "")
	pathFile := scriptFile(t, dir, "github-path", "")
	root := filepath.Join(dir, "tool cache")
	env := append(os.Environ(),
		"PATH="+fixtures+string(os.PathListSeparator)+os.Getenv("PATH"),
		"RUNNER_TOOL_CACHE="+root, "RUNNER_OS=Linux", "RUNNER_ARCH=X64",
		"GITHUB_OUTPUT="+output, "GITHUB_PATH="+pathFile, "GITHUB_ACTIONS=true",
		"GH_SETUP_GH_SETUP_VERSION=v1.2.3", "GH_SETUP_GITHUB_TOKEN=",
		"FAKE_BINARY="+binary, "CURL_LOG="+filepath.Join(dir, "curl-log"),
		"ARGUMENT_LOG="+filepath.Join(dir, "arguments"), "GH_SETUP_BIN="+binary,
		"GH_SETUP_BIN_DIR=", "GH_SETUP_FORCE=", "GH_SETUP_STRICT=",
		"GH_SETUP_SKIP_CONTENT_TYPE_CHECK=", "GH_SETUP_VERIFY_ATTESTATION=",
		"FAIL_DOWNLOAD=", "FAIL_BINARY=", "FAKE_CAPABLE=true",
	)
	return dir, env
}

func runScript(name string, env []string) ([]byte, error) {
	cmd := exec.Command("bash", name)
	cmd.Env = env
	return cmd.CombinedOutput()
}

func TestBootstrapReuse(t *testing.T) {
	dir, env := scriptEnvironment(t)
	for range 2 {
		output, err := runScript("install-gh-setup.sh", env)
		if err != nil {
			t.Fatalf("bootstrap: %v\n%s", err, output)
		}
	}
	log, err := os.ReadFile(filepath.Join(dir, "curl-log"))
	if err != nil || strings.Count(string(log), "\n") != 1 {
		t.Fatalf("bootstrap downloaded repeatedly: %q, %v", log, err)
	}
	root := filepath.Join(dir, "tool cache", "k1low", "gh-setup", "1.2.3", "x64")
	if _, err := os.Stat(root + ".complete"); err != nil {
		t.Fatal(err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "github-output"))
	if want := strings.Repeat("bin="+filepath.Join(root, "bin", "gh-setup")+"\n", 2); err != nil || string(out) != want {
		t.Fatalf("bootstrap output = %q, %v; want %q", out, err, want)
	}
	entries, err := os.ReadDir(filepath.Dir(root))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), "staging") || strings.HasSuffix(entry.Name(), ".lock") {
			t.Fatalf("bootstrap left temporary entry %s", entry.Name())
		}
	}
	if err := os.WriteFile(filepath.Join(root, "bin", "gh-setup"), []byte("corrupted"), 0600); err != nil {
		t.Fatal(err)
	}
	output, err := runScript("install-gh-setup.sh", env)
	if err != nil {
		t.Fatalf("bootstrap integrity recovery: %v\n%s", err, output)
	}
	log, err = os.ReadFile(filepath.Join(dir, "curl-log"))
	if err != nil || strings.Count(string(log), "\n") != 2 {
		t.Fatal("modified bootstrap binary reused")
	}
}

func TestBootstrapVersionParity(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "v1.2.3-rc.1+build.42", "v01.2.3", "v1.2.3-01",
		"v1.2.3-rc..1", "v1.2.3+build..1", "release-2026", "release/1", "a:b", "CON", "COM¹", "release.",
		strings.Repeat("a", 129), strings.Repeat("é", 65)} {
		t.Run(tag, func(t *testing.T) {
			dir, env := scriptEnvironment(t)
			output, err := runScript("install-gh-setup.sh", append(env, "GH_SETUP_GH_SETUP_VERSION="+tag))
			if err != nil {
				t.Fatalf("bootstrap tag %q: %v\n%s", tag, err, output)
			}
			version, err := setup.CacheVersion(tag)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "tool cache", "k1low", "gh-setup", version, "x64.complete")
			if _, err := os.Stat(path); err != nil {
				t.Fatalf("Bash/Go version mismatch for %s: %v", tag, err)
			}
		})
	}
}

func TestBootstrapConcurrentReuse(t *testing.T) {
	dir, env := scriptEnvironment(t)
	first := exec.Command("bash", "install-gh-setup.sh")
	first.Env = env
	second := exec.Command("bash", "install-gh-setup.sh")
	second.Env = env
	if err := first.Start(); err != nil {
		t.Fatal(err)
	}
	secondOutput, secondErr := second.CombinedOutput()
	firstErr := first.Wait()
	if firstErr != nil || secondErr != nil {
		t.Fatalf("concurrent bootstrap: %v, %v\n%s", firstErr, secondErr, secondOutput)
	}
	log, err := os.ReadFile(filepath.Join(dir, "curl-log"))
	if err != nil || strings.Count(string(log), "/releases/download/") != 1 {
		t.Fatalf("concurrent bootstrap downloaded twice: %q, %v", log, err)
	}
}

func TestBootstrapLatest(t *testing.T) {
	dir, env := scriptEnvironment(t)
	for range 2 {
		output, err := runScript("install-gh-setup.sh", append(env, "GH_SETUP_GH_SETUP_VERSION=latest"))
		if err != nil {
			t.Fatalf("bootstrap latest: %v\n%s", err, output)
		}
	}
	log, err := os.ReadFile(filepath.Join(dir, "curl-log"))
	if err != nil || strings.Count(string(log), "/releases/latest") != 2 ||
		strings.Count(string(log), "/releases/download/") != 1 {
		t.Fatalf("latest resolution/cache behavior: %q, %v", log, err)
	}
}

func TestBootstrapFailureDoesNotComplete(t *testing.T) {
	for _, failure := range []string{"FAIL_DOWNLOAD=true", "FAIL_BINARY=true", "RUNNER_ARCH=OTHER", "RUNNER_ARCH=X86"} {
		t.Run(failure, func(t *testing.T) {
			dir, env := scriptEnvironment(t)
			output, err := runScript("install-gh-setup.sh", append(env, failure))
			if err == nil {
				t.Fatalf("bootstrap succeeded with %s: %s", failure, output)
			}
			path := filepath.Join(dir, "tool cache", "k1low", "gh-setup", "1.2.3", "x64.complete")
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("failed bootstrap published completion: %v", err)
			}
		})
	}
}

func TestRunLiteralArguments(t *testing.T) {
	dir, env := scriptEnvironment(t)
	env = append(env, "GH_SETUP_REPO=example/tool", "GH_SETUP_MATCH=tool (linux|darwin).*",
		"GH_SETUP_BIN_MATCH=dir with space/tool$", "GH_SETUP_ATTESTATION_FLAGS=--owner=example --signer-workflow=release.yml",
		"GH_SETUP_FORCE=false", "GH_SETUP_STRICT=true", "GH_SETUP_VERIFY_ATTESTATION=true")
	output, err := runScript("run-gh-setup.sh", env)
	if err != nil {
		t.Fatalf("Action invocation: %v\n%s", err, output)
	}
	b, err := os.ReadFile(filepath.Join(dir, "arguments"))
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(b)), "\n")
	want := []string{
		"--match=tool (linux|darwin).*", "--bin-match=dir with space/tool$",
		"--attestation-flags=--owner=example --signer-workflow=release.yml",
		"--tool-cache", "--strict", "--verify-attestation",
	}
	for _, arg := range want {
		found := false
		for _, actual := range args {
			if actual == arg {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing literal argument %q: %q", arg, args)
		}
	}
	for _, actual := range args {
		if actual == "--force" {
			t.Fatal("force=false enabled Force")
		}
	}
}

func TestRunOlderVersion(t *testing.T) {
	dir, env := scriptEnvironment(t)
	env = append(env, "FAKE_CAPABLE=false")
	output, err := runScript("run-gh-setup.sh", env)
	if err == nil || !strings.Contains(string(output), "does not support Runner Tool Cache") {
		t.Fatalf("old binary compatibility diagnostic: %v\n%s", err, output)
	}
	if _, err := os.Stat(filepath.Join(dir, "arguments")); !os.IsNotExist(err) {
		t.Fatal("old binary was invoked without tool-cache support")
	}
	env = append(env, "GH_SETUP_BIN_DIR="+dir)
	output, err = runScript("run-gh-setup.sh", env)
	if err != nil {
		t.Fatalf("old binary explicit-directory opt-out: %v\n%s", err, output)
	}
	b, err := os.ReadFile(filepath.Join(dir, "github-path"))
	if err != nil || string(b) != fmt.Sprintln(dir) {
		t.Fatalf("legacy explicit directory PATH = %q, %v", b, err)
	}
}
