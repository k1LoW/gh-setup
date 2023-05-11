package gh

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/google/go-github/v52/github"
	"github.com/k1LoW/go-github-client/v52/factory"
	"golang.org/x/exp/slog"
)

type releaseAsset struct {
	ID          int64
	Name        string
	ContentType string
	DownloadURL string
}

type client struct {
	gc    *github.Client
	hc    *http.Client
	owner string
	repo  string
	token string
	v3ep  string
}

const (
	defaultV3Endpoint = "https://api.github.com"
	acceptHeader      = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.9"
)

func newClient(ctx context.Context, owner, repo string) (*client, error) {
	token, v3ep, _, _ := factory.GetTokenAndEndpoints()
	if token == "" {
		slog.Info("No credentials found, access without credentials", slog.String("endpoint", v3ep), slog.String("owner", owner), slog.String("repo", repo))
		return newNoAuthClient(ctx, owner, repo, v3ep)
	}
	slog.Info("Access with credentials", slog.String("endpoint", v3ep), slog.String("owner", owner), slog.String("repo", repo))
	gc, err := factory.NewGithubClient(factory.SkipAuth(true))
	if err != nil {
		return nil, err
	}
	if _, _, err := gc.Repositories.Get(ctx, owner, repo); err != nil {
		slog.Info("Authentication failed, access without credentials", slog.String("endpoint", v3ep), slog.String("owner", owner), slog.String("repo", repo))
		return newNoAuthClient(ctx, owner, repo, v3ep)
	}
	hc, err := api.DefaultHTTPClient()
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

func (c *client) getReleaseAssets(ctx context.Context, opt *AssetOption) ([]*releaseAsset, error) {
	if c.token != "" {
		assets, err := c.getReleaseAssetsWithAPI(ctx, opt)
		if err == nil {
			return assets, nil
		}
	}
	return c.getReleaseAssetsWithoutAPI(ctx, opt)
}

func (c *client) getReleaseAssetsWithoutAPI(ctx context.Context, opt *AssetOption) ([]*releaseAsset, error) {
	slog.Info("Get assets directly from the GitHub WebUI")
	if c.v3ep != defaultV3Endpoint {
		return nil, fmt.Errorf("not support for non-API access: %s", c.v3ep)
	}
	page := 1
	for {
		urls, err := c.getReleaseAssetsURLs(ctx, page)
		if err != nil {
			return nil, err
		}
		if len(urls) == 0 {
			break
		}
		if opt == nil || (opt.Version == "" || opt.Version == versionLatest) {
			return c.getReleaseAssetsViaURL(ctx, urls[0])
		} else {
			for _, url := range urls {
				if strings.HasSuffix(url, opt.Version) {
					return c.getReleaseAssetsViaURL(ctx, url)
				}
			}
		}
		page++
	}
	return nil, errors.New("no assets found")
}

func (c *client) getReleaseAssetsURLs(ctx context.Context, page int) ([]string, error) {
	u := fmt.Sprintf("https://github.com/%s/%s/releases?page=%d", c.owner, c.repo, page)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", acceptHeader)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Split(bufio.ScanLines)
	urls := []string{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, fmt.Sprintf("https://github.com/%s/%s/releases/expanded_assets/", c.owner, c.repo)) {
			splitted := strings.Split(line, `src="`)
			if len(splitted) == 2 {
				splitted2 := strings.Split(splitted[1], `"`)
				urls = append(urls, splitted2[0])
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return urls, nil
}

func (c *client) getReleaseAssetsViaURL(ctx context.Context, url string) ([]*releaseAsset, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", acceptHeader)
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Split(bufio.ScanLines)
	assets := []*releaseAsset{}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, "/download/") {
			splitted := strings.Split(line, `href="`)
			if len(splitted) == 2 {
				splitted2 := strings.Split(splitted[1], `"`)
				u := fmt.Sprintf("https://github.com%s", splitted2[0])
				splitted3 := strings.Split(splitted2[0], "/")
				name := splitted3[len(splitted3)-1]
				assets = append(assets, &releaseAsset{
					Name:        name,
					DownloadURL: u,
				})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}

func (c *client) getReleaseAssetsWithAPI(ctx context.Context, opt *AssetOption) ([]*releaseAsset, error) {
	slog.Info("Get assets using the GitHub API")
	var (
		r   *github.RepositoryRelease
		err error
	)
	if opt == nil || (opt.Version == "" || opt.Version == versionLatest) {
		r, _, err = c.gc.Repositories.GetLatestRelease(ctx, c.owner, c.repo)
		if err != nil {
			return nil, err
		}
	} else {
		r, _, err = c.gc.Repositories.GetReleaseByTag(ctx, c.owner, c.repo, opt.Version)
		if err != nil {
			return nil, err
		}
	}
	assets := []*releaseAsset{}
	for _, a := range r.Assets {
		assets = append(assets, &releaseAsset{
			ID:          a.GetID(),
			Name:        a.GetName(),
			ContentType: a.GetContentType(),
			DownloadURL: a.GetBrowserDownloadURL(),
		})
	}
	return assets, nil
}

func (c *client) downloadAsset(ctx context.Context, a *releaseAsset) ([]byte, error) {
	if c.token != "" {
		b, err := c.downloadAssetWithAPI(ctx, a)
		if err == nil {
			return b, nil
		}
	}
	return c.downloadAssetWithoutAPI(ctx, a)
}

func (c *client) downloadAssetWithoutAPI(ctx context.Context, a *releaseAsset) ([]byte, error) {
	slog.Info("Download asset directly from the GitHub WebUI")
	if a.DownloadURL == "" {
		return nil, errors.New("empty download URL")
	}
	u := a.DownloadURL
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download: %d %s", resp.StatusCode, string(b))
	}
	return b, nil
}

func (c *client) downloadAssetWithAPI(ctx context.Context, a *releaseAsset) ([]byte, error) {
	slog.Info("Download asset using the GitHub API")
	u := fmt.Sprintf("%s/repos/%s/%s/releases/assets/%d", c.v3ep, c.owner, c.repo, a.ID)
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download: %d %s", resp.StatusCode, string(b))
	}
	return b, nil
}
