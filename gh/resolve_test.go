package gh

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/go-github/v67/github"
)

func TestResolvedReleaseAPI(t *testing.T) {
	for _, version := range []string{"", "latest", "v1.2.3"} {
		t.Run(version, func(t *testing.T) {
			var downloads atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/download/") {
					downloads.Add(1)
					w.Header().Set("Content-Type", "application/octet-stream")
					fmt.Fprint(w, "\x00\x01\x02")
					return
				}
				want := "/repos/example/tool/releases/latest"
				if version == "v1.2.3" {
					want = "/repos/example/tool/releases/tags/v1.2.3"
				}
				if r.URL.Path != want {
					t.Errorf("release URL = %s; want %s", r.URL.Path, want)
				}
				w.Header().Set("Content-Type", "application/json")
				release := github.RepositoryRelease{
					TagName: github.String("v1.2.3"),
					Assets: []*github.ReleaseAsset{{
						ID: github.Int64(42), Name: github.String("tool_linux_amd64"),
						ContentType: github.String("application/octet-stream"), Size: github.Int(3),
						BrowserDownloadURL: github.String("http://" + r.Host + "/download/tool"),
					}},
				}
				if err := json.NewEncoder(w).Encode(release); err != nil {
					t.Error(err)
				}
			}))
			defer server.Close()
			gc := github.NewClient(server.Client())
			baseURL, err := url.Parse(server.URL + "/")
			if err != nil {
				t.Fatal(err)
			}
			gc.BaseURL = baseURL
			c := &client{gc: gc, hc: server.Client(), owner: "example", repo: "tool"}
			assets, err := c.getReleaseAssetsWithAPI(context.Background(), &AssetOption{Version: version})
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) != 1 || assets[0].Tag != "v1.2.3" || assets[0].ID != 42 || assets[0].Size != 3 {
				t.Fatalf("resolved assets = %+v", assets)
			}
			if downloads.Load() != 0 {
				t.Fatal("release resolution downloaded the asset")
			}
			selected := &ResolvedAsset{Asset: assets[0], client: c}
			fsys, err := selected.Materialize(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fs.ReadFile(fsys, "tool"); err != nil {
				t.Fatal(err)
			}
			if downloads.Load() != 1 {
				t.Fatal("materialization did not download the selected asset once")
			}
		})
	}
}

func TestResolvedReleaseHTML(t *testing.T) {
	for _, tag := range []string{"v1.2.3", "release/1", "release+build"} {
		t.Run(tag, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `<a href="/example/tool/releases/download/ignored-tag/tool_linux_amd64">tool</a>`)
			}))
			defer server.Close()
			c := &client{hc: server.Client()}
			assets, err := c.getReleaseAssetsViaURL(context.Background(), server.URL+"/example/tool/releases/expanded_assets/"+url.PathEscape(tag))
			if err != nil || len(assets) != 1 || assets[0].Tag != tag {
				t.Fatalf("resolved assets = %+v, %v; want tag %s", assets, err, tag)
			}
		})
	}
}

func TestReleaseTag(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/example/tool/releases/expanded_assets/v1.2.3":      "v1.2.3",
		"https://github.com/example/tool/releases/expanded_assets/release%2F1": "release/1",
		"https://github.com/example/tool/releases/expanded_assets/release%2B1": "release+1",
		"https://github.com/example/tool/releases/expanded_assets/release%251": "release%1",
	} {
		got, err := releaseTag(raw)
		if err != nil || got != want {
			t.Fatalf("releaseTag(%q) = %q, %v; want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{"https://github.com/example/tool", "https://github.com/example/tool/releases/expanded_assets/"} {
		if _, err := releaseTag(raw); err == nil {
			t.Fatalf("invalid release URL accepted: %s", raw)
		}
	}
}

func TestMaterializeVerificationFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "\x00\x01\x02")
	}))
	defer server.Close()
	r := &ResolvedAsset{
		Asset:   &releaseAsset{Name: "tool", ContentType: "application/octet-stream", DownloadURL: server.URL},
		client:  &client{hc: server.Client(), repo: "tool"},
		options: AssetOption{Checksum: "sha256:incorrect"},
	}
	if _, err := r.Materialize(context.Background()); err == nil {
		t.Fatal("incorrect checksum passed")
	}
}
