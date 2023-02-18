package gh

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cli/go-gh"
	"github.com/cli/go-gh/pkg/api"
	"github.com/google/go-github/v50/github"
	"github.com/k1LoW/go-github-client/v50/factory"
)

type releaseAsset struct {
	ID          int64
	Name        string
	ContentType string
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

func (c *client) getReleaseAssets(ctx context.Context, opt *AssetOption) ([]*releaseAsset, error) {
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
		})
	}
	return assets, nil
}

func (c *client) downloadAsset(ctx context.Context, a *releaseAsset) ([]byte, error) {
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
	return b, nil
}
