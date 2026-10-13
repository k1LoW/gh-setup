package cmd

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/k1LoW/gh-setup/gh"
	"github.com/k1LoW/gh-setup/setup"
)

func TestInstallRelease(t *testing.T) {
	for _, cache := range []bool{false, true} {
		name := "local"
		if cache {
			name = "cache"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			t.Setenv("RUNNER_TOOL_CACHE", root)
			request := setup.CacheRequest{
				Host: "github.com", Owner: "example", Repo: "tool",
				Tag: "v1.2.3", Arch: "x64", Asset: "asset",
			}
			calls := 0
			materialize := func(context.Context) (fs.FS, error) {
				calls++
				return fstest.MapFS{"tool": &fstest.MapFile{Data: []byte{0, 1, 2}}}, nil
			}
			options := &setup.Option{}
			if !cache {
				options.BinDir = root
			}
			result, err := installRelease(context.Background(), cache, request, options, materialize)
			if err != nil || result.Hit || calls != 1 {
				t.Fatalf("first installation = %+v, %v, calls=%d", result, err, calls)
			}
			if _, err := os.Stat(result.Bins["tool"]); err != nil {
				t.Fatal(err)
			}
			second, err := installRelease(context.Background(), cache, request, options, materialize)
			if cache {
				if err != nil || !second.Hit || calls != 1 {
					t.Fatalf("cache hit = %+v, %v, calls=%d", second, err, calls)
				}
			} else if err == nil || calls != 2 {
				t.Fatal("local installation no longer rejects existing files")
			}
			options.Force = true
			if _, err := installRelease(context.Background(), cache, request, options, materialize); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestActionsMissingEnvironmentBeforeResolution(t *testing.T) {
	oldOpt, oldSetup, oldOwner, oldCache := opt, sOpt, ownerrepo, toolCache
	t.Cleanup(func() { opt, sOpt, ownerrepo, toolCache = oldOpt, oldSetup, oldOwner, oldCache })
	opt, sOpt, ownerrepo, toolCache = &gh.AssetOption{}, &setup.Option{}, "example/tool", false
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("RUNNER_TOOL_CACHE", "")
	err := rootCmd.RunE(rootCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "RUNNER_TOOL_CACHE") {
		t.Fatalf("missing environment was not rejected before release resolution: %v", err)
	}
}

func TestInvalidPatternBeforeResolution(t *testing.T) {
	oldOpt, oldSetup, oldOwner, oldCache := opt, sOpt, ownerrepo, toolCache
	t.Cleanup(func() { opt, sOpt, ownerrepo, toolCache = oldOpt, oldSetup, oldOwner, oldCache })
	opt, sOpt, ownerrepo, toolCache = &gh.AssetOption{Match: "["}, &setup.Option{BinDir: t.TempDir()}, "example/tool", false
	t.Setenv("GITHUB_ACTIONS", "")
	if err := rootCmd.RunE(rootCmd, nil); err == nil || !strings.Contains(err.Error(), "regexp") {
		t.Fatalf("invalid pattern accepted: %v", err)
	}
}

func TestToolCacheCapability(t *testing.T) {
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	t.Cleanup(func() { rootCmd.SetOut(os.Stdout) })
	if err := rootCmd.Help(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "--tool-cache") {
		t.Fatal("Action cannot discover tool-cache capability")
	}
}

func TestExplicitBinDirDoesNotUseCache(t *testing.T) {
	root := filepath.Join(t.TempDir(), "unused")
	t.Setenv("RUNNER_TOOL_CACHE", root)
	dir := t.TempDir()
	result, err := installRelease(context.Background(), false, setup.CacheRequest{},
		&setup.Option{BinDir: dir}, func(context.Context) (fs.FS, error) {
			return fstest.MapFS{"tool": &fstest.MapFile{Data: []byte{0, 1, 2}}}, nil
		})
	if err != nil || result.BinDir != dir {
		t.Fatalf("explicit BinDir = %+v, %v", result, err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("explicit BinDir created a cache: %v", err)
	}
}
