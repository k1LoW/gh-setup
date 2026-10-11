/*
Copyright © 2023 Ken'ichiro Oyama <k1lowxb@gmail.com>

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
THE SOFTWARE.
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/k1LoW/gh-setup/gh"
	"github.com/k1LoW/gh-setup/setup"
	"github.com/k1LoW/gh-setup/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

var ownerrepo string

var (
	opt       = &gh.AssetOption{}
	sOpt      = &setup.Option{}
	verbose   bool
	toolCache bool
)

var rootCmd = &cobra.Command{
	Use:          "gh-setup",
	Short:        "Setup asset of Github Releases",
	Long:         `Setup asset of Github Releases.`,
	Args:         cobra.OnlyValidArgs,
	ValidArgs:    []string{"version"},
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) > 0 {
			cmd.Printf("gh-setup version %s\n", version.Version)
			return nil
		}
		setLogger(verbose)
		ctx := cmd.Context()
		actions := os.Getenv("GITHUB_ACTIONS") == "true"
		cache := (actions || toolCache) && sOpt.BinDir == ""
		if cache {
			if err := setup.ValidateActionsEnvironment(); err != nil {
				return err
			}
		}
		for _, pattern := range []string{opt.Match, sOpt.BinMatch} {
			if _, err := regexp.Compile(pattern); err != nil {
				return err
			}
		}
		host, owner, repo, err := gh.DetectHostOwnerRepo(ownerrepo)
		if err != nil {
			return err
		}
		// override env
		os.Setenv("GH_HOST", host)
		os.Unsetenv("GITHUB_API_URL")

		selection := setup.Selection{
			Match: opt.Match, BinMatch: sOpt.BinMatch, OS: opt.OS, Arch: opt.Arch,
			Strict: opt.Strict, SkipContentTypeCheck: opt.SkipContentTypeCheck,
		}
		assetOpt := *opt
		if cache {
			targetOS, targetArch, err := setup.RunnerTarget()
			if err != nil {
				return err
			}
			if assetOpt.OS == "" {
				assetOpt.OS = targetOS
			}
			if assetOpt.Arch == "" {
				assetOpt.Arch = targetArch
			}
		}
		resolved, err := gh.ResolveReleaseAsset(ctx, owner, repo, &assetOpt)
		if err != nil {
			return err
		}
		a := resolved.Asset
		cmd.Printf("Use %s\n", a.Name)
		asset, err := json.Marshal(a)
		if err != nil {
			return err
		}
		cacheArch := ""
		if cache {
			cacheArch, err = setup.CacheArch(os.Getenv("RUNNER_ARCH"))
			if err != nil {
				return err
			}
		}
		request := setup.CacheRequest{
			Host: host, Owner: owner, Repo: repo, Tag: a.Tag, Arch: cacheArch,
			Asset: string(asset), Selection: selection,
			Verification: setup.Verification{
				Checksum: opt.Checksum, VerifyAttestation: opt.VerifyAttestation,
				AttestationFlags: opt.AttestationFlags,
			},
		}
		result, err := installRelease(ctx, cache, request, sOpt, resolved.Materialize)
		if err != nil {
			return err
		}
		if actions || toolCache {
			if err := setup.AddPath(result.BinDir); err != nil {
				return err
			}
		}
		if result.Hit {
			cmd.Printf("Reuse tool cache: %s\n", result.BinDir)
		}
		cmd.Println("Setup binaries to executable path (PATH):")
		sources := make([]string, 0, len(result.Bins))
		for source := range result.Bins {
			sources = append(sources, source)
		}
		slices.Sort(sources)
		for _, source := range sources {
			cmd.Println(" ", source, "->", result.Bins[source])
		}
		return nil
	},
}

func installRelease(ctx context.Context, cache bool, request setup.CacheRequest, options *setup.Option,
	materialize func(context.Context) (fs.FS, error)) (*setup.CacheResult, error) {
	if cache {
		return setup.CacheBin(ctx, os.Getenv("RUNNER_TOOL_CACHE"), request, options, materialize)
	}
	fsys, err := materialize(ctx)
	if err != nil {
		return nil, err
	}
	bins, err := setup.Bin(fsys, options)
	if err != nil {
		return nil, err
	}
	if len(bins) == 0 {
		return nil, fmt.Errorf("setup failed: no binaries found in release asset")
	}
	result := &setup.CacheResult{Bins: bins}
	for _, bp := range bins {
		result.BinDir = filepath.Dir(bp)
		break
	}
	return result, nil
}

func Execute() {
	rootCmd.SetOut(os.Stdout)
	rootCmd.SetErr(os.Stderr)
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func setLogger(verbose bool) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	switch {
	case os.Getenv("DEBUG") != "" || os.Getenv("ACTIONS_STEP_DEBUG") != "":
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: true,
			Level:     slog.LevelDebug,
		}))
	case verbose:
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			AddSource: false,
			Level:     slog.LevelInfo,
		}))
	}
	slog.SetDefault(logger)
}

func init() {
	rootCmd.Flags().StringVarP(&ownerrepo, "repo", "R", "", "repository using the [HOST/]OWNER/REPO format")
	rootCmd.Flags().StringVarP(&opt.Version, "version", "v", "", "release version")
	rootCmd.Flags().StringVarP(&opt.OS, "os", "O", "", "specify OS of asset")
	rootCmd.Flags().StringVarP(&opt.Arch, "arch", "A", "", "specify arch of asset")
	rootCmd.Flags().StringVarP(&opt.Match, "match", "", "", "regexp to match asset name")
	rootCmd.Flags().StringVarP(&sOpt.BinDir, "bin-dir", "", "", "bin directory for setup")
	rootCmd.Flags().BoolVar(&toolCache, "tool-cache", false, "use Runner Tool Cache (automatic in GitHub Actions unless --bin-dir is set)")
	rootCmd.Flags().StringVarP(&sOpt.BinMatch, "bin-match", "", "", "regexp to match bin path in asset")
	rootCmd.Flags().StringVarP(&opt.Checksum, "checksum", "", "", "checksum of asset")
	rootCmd.Flags().BoolVarP(&sOpt.Force, "force", "f", false, "enable force setup")
	rootCmd.Flags().BoolVarP(&opt.Strict, "strict", "", false, "require strict match")
	rootCmd.Flags().BoolVarP(&opt.SkipContentTypeCheck, "skip-content-type-check", "", false, "skip check content-type of assets")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "", false, "show verbose log")
	rootCmd.Flags().BoolVarP(&opt.VerifyAttestation, "verify-attestation", "", false, "enable attestation verification using 'gh attestation verify'")
	rootCmd.Flags().StringVarP(&opt.AttestationFlags, "attestation-flags", "", "", "additional flags to pass to 'gh attestation verify' (e.g. \"--owner=k1LoW\")")
}
