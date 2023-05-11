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
	"fmt"
	"io"
	"os"

	"github.com/k1LoW/gh-setup/gh"
	"github.com/k1LoW/gh-setup/setup"
	"github.com/k1LoW/gh-setup/version"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slog"
)

var ownerrepo string

var (
	opt     = &gh.AssetOption{}
	sOpt    = &setup.Option{}
	verbose bool
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
		ctx := context.Background()
		setLogger(verbose)
		host, owner, repo, err := gh.DetectHostOwnerRepo(ownerrepo)
		if err != nil {
			return err
		}
		// override env
		os.Setenv("GH_HOST", host)
		os.Unsetenv("GITHUB_API_URL")

		a, fsys, err := gh.GetReleaseAsset(ctx, owner, repo, opt)
		if err != nil {
			return err
		}
		cmd.Printf("Use %s\n", a.Name)
		m, err := setup.Bin(fsys, sOpt)
		if err != nil {
			return err
		}
		if len(m) == 0 {
			return fmt.Errorf("setup failed: %s", a.Name)
		}
		cmd.Println("Setup binaries to executable path (PATH):")
		for b, bp := range m {
			cmd.Println(" ", b, "->", bp)
		}
		return nil
	},
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
	rootCmd.Flags().StringVarP(&sOpt.BinMatch, "bin-match", "", "", "regexp to match bin path in asset")
	rootCmd.Flags().BoolVarP(&sOpt.Force, "force", "f", false, "enable force setup")
	rootCmd.Flags().BoolVarP(&opt.Strict, "strict", "", false, "require strict match")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "", false, "show verbose log")
}
