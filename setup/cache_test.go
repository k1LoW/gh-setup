package setup

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func testRequest() CacheRequest {
	return CacheRequest{
		Host: "github.com", Owner: "Example", Repo: "Tool",
		Tag: "v1.2.3", Arch: "x64", Asset: "asset-1",
	}
}

func testAsset() fs.FS {
	return fstest.MapFS{
		"dist/tool": &fstest.MapFile{Data: []byte{0, 1, 2, 3, 4}},
	}
}

func writeTestFile(t *testing.T, path string, b []byte) {
	t.Helper()
	if err := os.WriteFile(path, b, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0755); err != nil {
		t.Fatal(err)
	}
}

func TestCacheVersion(t *testing.T) {
	tests := map[string]string{
		"v1.2.3": "1.2.3", "1.2.3": "1.2.3",
		"v1.2.3-rc.1+build.42": "1.2.3-rc.1+build.42",
		"v01.2.3":              "v01.2.3", "v1.2.3-01": "v1.2.3-01",
		"v1.2.3-rc..1": "v1.2.3-rc..1", "v1.2.3+build..1": "v1.2.3+build..1",
		"release-2026": "release-2026", "main": "main", "v1.2": "v1.2",
	}
	for tag, want := range tests {
		t.Run(tag, func(t *testing.T) {
			got, err := CacheVersion(tag)
			if err != nil || got != want {
				t.Fatalf("CacheVersion(%q) = %q, %v; want %q", tag, got, err, want)
			}
		})
	}
	for _, tag := range []string{"../bad", "release/1", `release\1`, "a:b", "a\nb", "CON", "nul.txt",
		"LPT1", "COM¹", ".", "..", "release.", "release ", strings.Repeat("a", 129)} {
		t.Run(tag, func(t *testing.T) {
			got, err := CacheVersion(tag)
			if err != nil || got != "tag-"+digest([]byte(tag)) {
				t.Fatalf("unsafe tag %q = %q, %v", tag, got, err)
			}
		})
	}
	if _, err := CacheVersion(""); err == nil {
		t.Fatal("empty tag accepted")
	}
}

func TestCacheArch(t *testing.T) {
	for runner, want := range map[string]string{"X86": "ia32", "X64": "x64", "ARM": "arm", "ARM64": "arm64"} {
		got, err := CacheArch(runner)
		if err != nil || got != want {
			t.Fatalf("%s = %s, %v", runner, got, err)
		}
	}
	for _, runner := range []string{"", "amd64", "x64", "OTHER"} {
		if _, err := CacheArch(runner); err == nil {
			t.Fatalf("unsupported runner architecture %q accepted", runner)
		}
	}
}

func TestCachePaths(t *testing.T) {
	root := t.TempDir()
	request := testRequest()
	path, err := request.Path(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "example", "tool", "1.2.3", "x64"); path != want {
		t.Fatalf("path = %s; want %s", path, want)
	}
	variants := []CacheRequest{request, request, request, request, request, request, request}
	variants[0].Selection.Match = "zip$"
	variants[1].Selection.BinMatch = "tool$"
	variants[2].Selection.OS = "linux"
	variants[3].Selection.Arch = "arm64"
	variants[4].Selection.Strict = true
	variants[5].Selection.SkipContentTypeCheck = true
	variants[6].Host = "github.example.com"
	seen := map[string]bool{path: true}
	for _, variant := range variants {
		p, err := variant.Path(root)
		if err != nil {
			t.Fatal(err)
		}
		relative, err := filepath.Rel(root, filepath.Join(p, "bin"))
		if err != nil {
			t.Fatal(err)
		}
		if depth := len(strings.Split(relative, string(filepath.Separator))); depth != 5 {
			t.Fatalf("bin depth = %d: %s", depth, relative)
		}
		if seen[p] {
			t.Fatalf("variant collision: %s", p)
		}
		seen[p] = true
	}
	for _, component := range []string{"..", "../outside", "CON", "a/b"} {
		request.Owner = component
		if _, err := request.Path(root); err == nil {
			t.Fatalf("invalid owner %q accepted", component)
		}
	}
}

func TestCacheLifecycle(t *testing.T) {
	root := t.TempDir()
	request := testRequest()
	calls := 0
	materialize := func(context.Context) (fs.FS, error) {
		calls++
		return testAsset(), nil
	}
	install := func(opt *Option) *CacheResult {
		t.Helper()
		result, err := CacheBin(context.Background(), root, request, opt, materialize)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	result := install(nil)
	if result.Hit || calls != 1 || len(result.Bins) != 1 {
		t.Fatalf("unexpected first installation: %+v; downloads=%d", result, calls)
	}
	path, err := request.Path(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".complete"); err != nil {
		t.Fatal(err)
	}
	binary := result.Bins["dist/tool"]
	info, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	result = install(nil)
	after, err := os.Stat(binary)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Hit || calls != 1 || !info.ModTime().Equal(after.ModTime()) {
		t.Fatal("cache hit downloaded or rewrote the binary")
	}
	if result = install(&Option{Force: true}); result.Hit || calls != 2 {
		t.Fatal("Force did not refresh")
	}
	failure := errors.New("verification failed")
	if _, err := CacheBin(context.Background(), root, request, &Option{Force: true},
		func(context.Context) (fs.FS, error) { return nil, failure }); !errors.Is(err, failure) {
		t.Fatalf("got error %v; want verification error", err)
	}
	if result = install(nil); !result.Hit || calls != 2 {
		t.Fatal("failed Force refresh damaged the completed entry")
	}
	if err := os.Remove(path + ".complete"); err != nil {
		t.Fatal(err)
	}
	if result = install(nil); result.Hit || calls != 3 {
		t.Fatal("incomplete installation reused")
	}
	writeTestFile(t, binary, []byte("modified"))
	if result = install(nil); result.Hit || calls != 4 {
		t.Fatal("modified binary reused")
	}
	if err := os.Remove(binary); err != nil {
		t.Fatal(err)
	}
	if result = install(nil); result.Hit || calls != 5 {
		t.Fatal("missing binary reused")
	}
	request.Asset = "asset-2"
	if result = install(nil); result.Hit || calls != 6 {
		t.Fatal("changed asset reused")
	}
	for _, policy := range []Verification{
		{Checksum: "sha256:one"},
		{Checksum: "sha256:two"},
		{VerifyAttestation: true, AttestationFlags: "--owner=example"},
		{VerifyAttestation: true, AttestationFlags: "--owner=other"},
	} {
		request.Verification = policy
		before := calls
		if result = install(nil); result.Hit || calls != before+1 {
			t.Fatalf("new verification policy reused a weaker cache: %+v", policy)
		}
		if result = install(nil); !result.Hit || calls != before+1 {
			t.Fatal("identically verified entry was not reused")
		}
	}
}

func TestCacheInvalidEntries(t *testing.T) {
	for _, kind := range []string{"marker-only", "empty", "no-manifest", "invalid-manifest", "collision", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			request := testRequest()
			path, err := request.Path(root)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path+".complete", nil)
			if kind != "marker-only" {
				if err := os.MkdirAll(filepath.Join(path, "bin"), 0755); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "empty" || kind == "collision" {
				recorded := request.normalized()
				if kind == "collision" {
					recorded.Tag = "different"
				}
				b, err := json.Marshal(cacheManifest{Schema: cacheSchema, Request: recorded})
				if err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(path, "gh-setup.json"), b)
			}
			if kind == "invalid-manifest" {
				writeTestFile(t, filepath.Join(path, "gh-setup.json"), []byte("not json"))
			}
			if kind == "symlink" {
				if err := os.Symlink(t.TempDir(), filepath.Join(path, "gh-setup.json")); err != nil {
					if runtime.GOOS == "windows" {
						t.Skip("symlinks unavailable")
					}
					t.Fatal(err)
				}
			}
			result, err := CacheBin(context.Background(), root, request, nil,
				func(context.Context) (fs.FS, error) { return testAsset(), nil })
			if kind == "invalid-manifest" || kind == "collision" || kind == "symlink" {
				if err == nil {
					t.Fatalf("%s unexpectedly accepted", kind)
				}
			} else if err != nil || result.Hit {
				t.Fatalf("incomplete entry not reinstalled: %+v, %v", result, err)
			}
		})
	}
}

func TestCacheConcurrentInstall(t *testing.T) {
	root := t.TempDir()
	request := testRequest()
	var calls atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			result, err := CacheBin(context.Background(), root, request, nil,
				func(context.Context) (fs.FS, error) {
					calls.Add(1)
					time.Sleep(20 * time.Millisecond)
					return testAsset(), nil
				})
			if err != nil {
				t.Error(err)
			} else if len(result.Bins) != 1 {
				t.Error("missing installed binary")
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("concurrent installation downloaded %d times", calls.Load())
	}
}

func TestCacheLockCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x64.lock")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := acquireCacheLock(ctx, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("lock cancellation: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("removed another installer's lock")
	}
}

func TestPublishCacheRollback(t *testing.T) {
	parent := t.TempDir()
	path := filepath.Join(parent, "x64")
	if err := os.Mkdir(path, 0755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(path, "original"), []byte("original"))
	writeTestFile(t, path+".complete", nil)
	staging := filepath.Join(parent, "staging")
	if err := os.MkdirAll(filepath.Join(staging, "executables"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := publishCache(path, staging); err == nil {
		t.Fatal("publication without manifest succeeded")
	}
	b, err := os.ReadFile(filepath.Join(path, "original"))
	if err != nil || string(b) != "original" {
		t.Fatalf("original installation lost: %q, %v", b, err)
	}
	if _, err := os.Stat(path + ".complete"); err != nil {
		t.Fatal("original completion marker lost")
	}
}

func TestCacheSymlinkPath(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "example")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks unavailable")
		}
		t.Fatal(err)
	}
	if _, err := testRequest().Path(root); err == nil {
		t.Fatal("symlink in repository path accepted")
	}
}

func TestCacheBinaryVariants(t *testing.T) {
	root := t.TempDir()
	fsys := fstest.MapFS{
		"dist/one": &fstest.MapFile{Data: []byte{0, 1}},
		"dist/two": &fstest.MapFile{Data: []byte{0, 2}},
	}
	materialize := func(context.Context) (fs.FS, error) { return fsys, nil }
	for _, name := range []string{"one", "two"} {
		request := testRequest()
		request.Selection.BinMatch = name + "$"
		result, err := CacheBin(context.Background(), root, request, nil, materialize)
		if err != nil || len(result.Bins) != 1 {
			t.Fatalf("binary variant %s: %+v, %v", name, result, err)
		}
		if _, ok := result.Bins["dist/"+name]; !ok {
			t.Fatalf("wrong binary installed for variant %s", name)
		}
	}
	for _, name := range []string{"one", "two"} {
		request := testRequest()
		request.Selection.BinMatch = name + "$"
		result, err := CacheBin(context.Background(), root, request, nil,
			func(context.Context) (fs.FS, error) { t.Fatal("variant cache miss"); return nil, nil })
		if err != nil || !result.Hit {
			t.Fatalf("variant overwritten: %+v, %v", result, err)
		}
	}
}

func TestCacheFailedInstallation(t *testing.T) {
	for _, kind := range []string{"verification", "no-binaries", "duplicate-basename"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			request := testRequest()
			_, err := CacheBin(context.Background(), root, request, nil,
				func(context.Context) (fs.FS, error) {
					switch kind {
					case "verification":
						return nil, errors.New("checksum failed")
					case "no-binaries":
						return fstest.MapFS{}, nil
					default:
						return fstest.MapFS{
							"a/tool": &fstest.MapFile{Data: []byte{0, 1}},
							"b/tool": &fstest.MapFile{Data: []byte{0, 2}},
						}, nil
					}
				})
			if err == nil {
				t.Fatal("failed installation reported success")
			}
			path, err := request.Path(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path + ".complete"); !os.IsNotExist(err) {
				t.Fatal("failed installation published completion")
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 0 {
				t.Fatalf("failed installation left staging files: %+v", entries)
			}
		})
	}
}

func TestRunnerTarget(t *testing.T) {
	for runnerOS, wantOS := range map[string]string{"Linux": "linux", "macOS": "darwin", "Windows": "windows"} {
		for runnerArch, wantArch := range map[string]string{"X86": "386", "X64": "amd64", "ARM": "arm", "ARM64": "arm64"} {
			t.Setenv("RUNNER_OS", runnerOS)
			t.Setenv("RUNNER_ARCH", runnerArch)
			gotOS, gotArch, err := RunnerTarget()
			if err != nil || gotOS != wantOS || gotArch != wantArch {
				t.Fatalf("target %s/%s = %s/%s, %v", runnerOS, runnerArch, gotOS, gotArch, err)
			}
		}
	}
	t.Setenv("RUNNER_OS", "Unsupported")
	if _, _, err := RunnerTarget(); err == nil {
		t.Fatal("unsupported runner OS accepted")
	}
}

func TestActionsEnvironmentAndPath(t *testing.T) {
	root := t.TempDir()
	pathFile := filepath.Join(t.TempDir(), "github-path")
	writeTestFile(t, pathFile, nil)
	t.Setenv("RUNNER_TOOL_CACHE", root)
	t.Setenv("RUNNER_ARCH", "X64")
	t.Setenv("RUNNER_OS", "Linux")
	t.Setenv("GITHUB_PATH", pathFile)
	if err := ValidateActionsEnvironment(); err != nil {
		t.Fatal(err)
	}
	binDir := filepath.Join(root, "example", "tool", "1.2.3", "x64", "bin")
	for range 2 {
		if err := AddPath(binDir); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(pathFile)
	if err != nil || string(b) != strings.Repeat(binDir+"\n", 2) {
		t.Fatalf("GITHUB_PATH = %q, %v", b, err)
	}
	if err := AddPath("bad\npath"); err == nil {
		t.Fatal("newline directory accepted")
	}
	for _, name := range []string{"RUNNER_TOOL_CACHE", "RUNNER_ARCH", "RUNNER_OS", "GITHUB_PATH"} {
		t.Run(name, func(t *testing.T) {
			t.Setenv(name, "")
			if err := ValidateActionsEnvironment(); err == nil {
				t.Fatalf("missing %s accepted", name)
			}
		})
	}
}
