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
	"errors"
	"fmt"
	"os"

	"github.com/k1LoW/gh-setup/gh"
	"github.com/k1LoW/gh-setup/setup"
	"github.com/k1LoW/gh-setup/version"
	"github.com/spf13/cobra"
)

var (
	ownerrepo string
	binDir    string
	force     bool
)

var opt = &gh.AssetOption{}

var rootCmd = &cobra.Command{
	Use:     "gh-setup",
	Short:   "Setup asset of Github Releases",
	Long:    `Setup asset of Github Releases.`,
	Version: version.Version,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		host, owner, repo, err := gh.DetectHostOwnerRepo(ownerrepo)
		if err != nil {
			return err
		}
		os.Setenv("GH_HOST", host) // override GH_HOST
		if opt != nil && opt.Match != "" {
			if opt.OS != "" || opt.Arch != "" {
				return errors.New("--match and --os/--arch options cannot be used together")
			}
		}

		a, fsys, err := gh.GetReleaseAsset(ctx, owner, repo, opt)
		if err != nil {
			return err
		}
		cmd.Printf("Use %s\n", a.GetName())
		m, err := setup.Bin(fsys, binDir, force)
		if err != nil {
			return err
		}
		if len(m) == 0 {
			return fmt.Errorf("setup failed: %s", a.GetName())
		}
		cmd.Println("Setup binaries to executable path (PATH):")
		for b, bp := range m {
			cmd.Println(" ", b, "->", bp)
		}
		return nil
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringVarP(&ownerrepo, "repo", "R", "", "repository using the [HOST/]OWNER/REPO format")
	rootCmd.Flags().StringVarP(&binDir, "bin-dir", "", "", "bin directory for setup")
	rootCmd.Flags().BoolVarP(&force, "force", "f", false, "enable force setup")
	rootCmd.Flags().StringVarP(&opt.Version, "release-version", "V", "", "release version")
	rootCmd.Flags().StringVarP(&opt.OS, "os", "O", "", "specify OS of asset")
	rootCmd.Flags().StringVarP(&opt.Arch, "arch", "A", "", "specify arch of asset")
	rootCmd.Flags().StringVarP(&opt.Match, "match", "", "", "match string for asset")
}
