package gh

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cli/go-gh"
	"github.com/cli/go-gh/pkg/api"
	"github.com/google/go-github/v50/github"
	"github.com/k1LoW/go-github-client/v50/factory"
	"github.com/nlepage/go-tarfs"
)

var osDict = map[string][]string{
	"darwin":  {"darwin", "macos"},
	"windows": {"windows"},
	"linux":   {"linux"},
}

var archDict = map[string][]string{
	"amd64": {"amd64", "x86_64", "x64"},
	"arm64": {"arm64", "aarch64"},
}

var supportContentType = []string{
	// zip
	"application/zip",
	"application/x-zip-compressed",
	// tar.gz
	"application/gzip",
	// binary
	"application/octet-stream",
}

type AssetOption struct {
	Match   string
	Version string
	OS      string
	Arch    string
}

func GetReleaseAsset(ctx context.Context, owner, repo string, opt *AssetOption) (*github.ReleaseAsset, fs.FS, error) {
	c, err := factory.NewGithubClient()
	if err != nil {
		return nil, nil, err
	}
	var r *github.RepositoryRelease
	if opt != nil && opt.Version == "" {
		r, _, err = c.Repositories.GetLatestRelease(ctx, owner, repo)
		if err != nil {
			return nil, nil, err
		}
	} else {
		r, _, err = c.Repositories.GetReleaseByTag(ctx, owner, repo, opt.Version)
		if err != nil {
			return nil, nil, err
		}
	}
	a, err := detectAsset(r.Assets, opt)
	if err != nil {
		return nil, nil, err
	}
	fsys, err := makeFS(owner, repo, a)
	if err != nil {
		return nil, nil, err
	}
	return a, fsys, nil
}

func detectAsset(assets []*github.ReleaseAsset, opt *AssetOption) (*github.ReleaseAsset, error) {
	var (
		od, ad, om *regexp.Regexp
	)
	if opt != nil && opt.Match != "" {
		om = regexp.MustCompile(opt.Match)
	}
	if opt != nil && opt.OS != "" {
		od = getDictRegexp(opt.OS, osDict)
	} else {
		od = getDictRegexp(runtime.GOOS, osDict)
	}
	if opt != nil && opt.Arch != "" {
		ad = getDictRegexp(opt.Arch, archDict)
	} else {
		ad = getDictRegexp(runtime.GOARCH, archDict)
	}

	type assetScore struct {
		asset *github.ReleaseAsset
		score int
	}
	assetScores := []*assetScore{}
	for _, a := range assets {
		if om != nil && om.MatchString(a.GetName()) {
			return a, nil
		}
		if !contains(supportContentType, a.GetContentType()) {
			continue
		}
		as := &assetScore{
			asset: a,
			score: 0,
		}
		assetScores = append(assetScores, as)
		// os
		if od.MatchString(a.GetName()) {
			as.score += 7
		}
		// arch
		if ad.MatchString(a.GetName()) {
			as.score += 3
		}
		// content type
		if a.GetContentType() == "application/octet-stream" {
			as.score += 1
		}
	}
	if len(assetScores) == 0 {
		return nil, errors.New("assets not found")
	}

	sort.Slice(assetScores, func(i, j int) bool {
		return assetScores[i].score > assetScores[j].score
	})

	return assetScores[0].asset, nil
}

func getDictRegexp(key string, dict map[string][]string) *regexp.Regexp {
	for k, d := range dict {
		if strings.ToLower(key) == k {
			return regexp.MustCompile(fmt.Sprintf("(?i)(%s)", strings.Join(d, "|")))
		}
	}
	return regexp.MustCompile(fmt.Sprintf("(?i)(%s)", strings.ToLower(key)))
}

func makeFS(owner, repo string, a *github.ReleaseAsset) (fs.FS, error) {
	client, err := gh.HTTPClient(&api.ClientOptions{
		Headers: map[string]string{
			"Accept": "application/octet-stream",
		},
	})
	if err != nil {
		return nil, err
	}
	_, v3ep, _, _ := factory.GetTokenAndEndpoints()
	u := fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", v3ep, owner, repo, a.GetID())
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	switch a.GetContentType() {
	case "application/zip", "application/x-zip-compressed":
		return zip.NewReader(bytes.NewReader(b), int64(len(b)))
	case "application/gzip":
		gr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		if strings.HasSuffix(a.GetName(), ".tar.gz") {
			fsys, err := tarfs.New(gr)
			if err != nil {
				return nil, err
			}
			return fsys, nil
		} else {
			b, err := io.ReadAll(gr)
			if err != nil {
				return nil, err
			}
			fsys := fstest.MapFS{}
			fsys[repo] = &fstest.MapFile{
				Data:    b,
				Mode:    fs.ModePerm,
				ModTime: time.Now(),
			}
			return fsys, nil
		}
	case "application/octet-stream":
		fsys := fstest.MapFS{}
		fsys[repo] = &fstest.MapFile{
			Data:    b,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
		}
		return fsys, nil
	default:
		return nil, fmt.Errorf("unsupport content type: %s", a.GetContentType())
	}
}

func contains(s []string, e string) bool {
	for _, v := range s {
		if e == v {
			return true
		}
	}
	return false
}

func DetectOwnerRepo(ownerrepo string) (string, string, error) {
	var owner, repo string
	if ownerrepo == "" {
		r, err := gh.CurrentRepository()
		if err != nil {
			return "", "", err
		}
		owner = r.Owner()
		repo = r.Name()
	} else {
		splitted := strings.Split(ownerrepo, "/")
		if len(splitted) != 2 {
			return "", "", fmt.Errorf("invalid repo: %s", ownerrepo)
		}
		owner = splitted[0]
		repo = splitted[1]
	}
	return owner, repo, nil
}
