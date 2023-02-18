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
	"log"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cli/go-gh"
	"github.com/cli/go-gh/pkg/api"
	"github.com/cli/go-gh/pkg/repository"
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
	Strict  bool
}

func GetReleaseAsset(ctx context.Context, owner, repo string, opt *AssetOption) (*github.ReleaseAsset, fs.FS, error) {
	const versionLatest = "latest"
	c, err := newClient(ctx, owner, repo)
	if err != nil {
		return nil, nil, err
	}
	var r *github.RepositoryRelease
	if opt != nil && (opt.Version == "" || opt.Version == versionLatest) {
		r, _, err = c.gc.Repositories.GetLatestRelease(ctx, owner, repo)
		if err != nil {
			return nil, nil, err
		}
	} else {
		r, _, err = c.gc.Repositories.GetReleaseByTag(ctx, owner, repo, opt.Version)
		if err != nil {
			return nil, nil, err
		}
	}
	a, err := detectAsset(r.Assets, opt)
	if err != nil {
		return nil, nil, err
	}
	b, err := c.downloadAsset(ctx, a)
	if err != nil {
		return nil, nil, err
	}
	fsys, err := makeFS(ctx, b, repo, a.GetName(), []string{a.GetContentType(), http.DetectContentType(b)})
	if err != nil {
		return nil, nil, err
	}
	return a, fsys, nil
}

func DetectHostOwnerRepo(ownerrepo string) (string, string, string, error) {
	var host, owner, repo string
	if ownerrepo == "" {
		r, err := gh.CurrentRepository()
		if err != nil {
			return "", "", "", err
		}
		host = r.Host()
		owner = r.Owner()
		repo = r.Name()
	} else {
		r, err := repository.Parse(ownerrepo)
		if err != nil {
			return "", "", "", err
		}
		host = r.Host()
		owner = r.Owner()
		repo = r.Name()
	}
	return host, owner, repo, nil
}

func detectAsset(assets []*github.ReleaseAsset, opt *AssetOption) (*github.ReleaseAsset, error) {
	var (
		od, ad, om *regexp.Regexp
		err        error
	)
	if opt != nil && opt.Match != "" {
		om, err = regexp.Compile(opt.Match)
		if err != nil {
			return nil, err
		}
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
	if opt.Strict && om != nil {
		return nil, fmt.Errorf("no matching assets found: %s", opt.Match)
	}
	if len(assetScores) == 0 {
		return nil, errors.New("no matching assets found")
	}

	sort.Slice(assetScores, func(i, j int) bool {
		return assetScores[i].score > assetScores[j].score
	})

	if opt.Strict && assetScores[0].score < 10 {
		return nil, fmt.Errorf("no matching assets found for OS/Arch: %s/%s", opt.OS, opt.Arch)
	}

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

func contains(s []string, e string) bool {
	for _, v := range s {
		if e == v {
			return true
		}
	}
	return false
}

type client struct {
	gc    *github.Client
	hc    *http.Client
	owner string
	repo  string
	token string
	v3ep  string
}

func newClient(ctx context.Context, owner, repo string) (*client, error) {
	token, v3ep, _, _ := factory.GetTokenAndEndpoints()
	if token == "" {
		log.Println("No credentials found, access without credentials")
		return newNoAuthClient(ctx, owner, repo, v3ep)
	}
	log.Println("Access with credentials")
	gc, err := factory.NewGithubClient(factory.SkipAuth(true))
	if err != nil {
		return nil, err
	}
	if _, _, err := gc.Repositories.Get(ctx, owner, repo); err != nil {
		log.Println("Authentication failed, access without credentials")
		return newNoAuthClient(ctx, owner, repo, v3ep)
	}
	hc, err := gh.HTTPClient(&api.ClientOptions{})
	if err != nil {
		return nil, err
	}
	c := &client{
		owner: owner,
		repo:  repo,
		token: token,
		v3ep:  v3ep,
		gc:    gc,
		hc:    hc,
	}
	return c, nil
}

func newNoAuthClient(ctx context.Context, owner, repo, v3ep string) (*client, error) {
	gc, err := factory.NewGithubClient(factory.SkipAuth(true))
	if err != nil {
		return nil, err
	}
	hc := &http.Client{
		Timeout:   30 * time.Second,
		Transport: http.DefaultTransport.(*http.Transport).Clone(),
	}
	c := &client{
		owner: owner,
		repo:  repo,
		v3ep:  v3ep,
		gc:    gc,
		hc:    hc,
	}
	return c, nil
}

func makeFS(ctx context.Context, b []byte, repo, name string, contentTypes []string) (fs.FS, error) {
	log.Println("asset content type:", contentTypes)
	switch {
	case matchContentTypes([]string{"application/zip", "application/x-zip-compressed"}, contentTypes):
		return zip.NewReader(bytes.NewReader(b), int64(len(b)))
	case matchContentTypes([]string{"application/gzip", "application/x-gzip"}, contentTypes):
		gr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		if strings.HasSuffix(name, ".tar.gz") {
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
	case matchContentTypes([]string{"application/octet-stream"}, contentTypes):
		fsys := fstest.MapFS{}
		fsys[repo] = &fstest.MapFile{
			Data:    b,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
		}
		return fsys, nil
	default:
		return nil, fmt.Errorf("unsupport content types: %s", contentTypes)
	}
}

func (c *client) downloadAsset(ctx context.Context, a *github.ReleaseAsset) ([]byte, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", c.v3ep, c.owner, c.repo, a.GetID())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", "application/octet-stream")
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func matchContentTypes(m, ct []string) bool {
	for _, v := range m {
		for _, vv := range ct {
			if v == vv {
				return true
			}
		}
	}
	return false
}
