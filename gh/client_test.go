package gh

import (
	"bufio"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGetReleaseAssetsWithoutAPI(t *testing.T) {
	tests := []struct {
		owner string
		repo  string
		opt   *AssetOption
	}{
		{"k1LoW", "tbls", nil},
	}
	ctx := context.Background()
	for _, tt := range tests {
		c, err := newClient(ctx, tt.owner, tt.repo)
		if err != nil {
			t.Error(err)
			continue
		}
		assets, err := c.getReleaseAssetsWithoutAPI(ctx, tt.opt)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(assets) == 0 {
			t.Error("want assets")
		}
	}
}

// longLine is longer than bufio.MaxScanTokenSize. The GitHub WebUI embeds such lines
// (inline scripts) in the release pages, and they must not stop the parsing.
var longLine = fmt.Sprintf(`<script type="application/json">%s</script>`, strings.Repeat("x", bufio.MaxScanTokenSize))

func TestParseReleaseAssetsURLs(t *testing.T) {
	c := &client{owner: "k1LoW", repo: "octocov"}
	body := strings.Join([]string{
		longLine,
		`  <include-fragment src="https://github.com/k1LoW/octocov/releases/expanded_assets/v0.75.12" data-test-selector="lazy-assets">`,
		`  <include-fragment src="https://github.com/k1LoW/octocov/releases/expanded_assets/v0.75.11" data-test-selector="lazy-assets">`,
		`  <include-fragment src="https://github.com/k1LoW/other/releases/expanded_assets/v1.0.0">`,
	}, "\n")
	got := c.parseReleaseAssetsURLs(body)
	want := []string{
		"https://github.com/k1LoW/octocov/releases/expanded_assets/v0.75.12",
		"https://github.com/k1LoW/octocov/releases/expanded_assets/v0.75.11",
	}
	if diff := cmp.Diff(got, want, nil); diff != "" {
		t.Error(diff)
	}
}

func TestParseReleaseAssets(t *testing.T) {
	body := strings.Join([]string{
		longLine,
		`        <a href="/k1LoW/octocov/releases/download/v0.75.12/octocov_v0.75.12_darwin_arm64.zip" rel="nofollow" class="Truncate">`,
		`        <a href="/k1LoW/octocov/releases/download/v0.75.12/checksums.txt" rel="nofollow" class="Truncate">`,
		`        <a href="/k1LoW/octocov/releases/tag/v0.75.12">v0.75.12</a>`,
	}, "\n")
	got := parseReleaseAssets(body)
	want := []*releaseAsset{
		{
			Name:        "octocov_v0.75.12_darwin_arm64.zip",
			DownloadURL: "https://github.com/k1LoW/octocov/releases/download/v0.75.12/octocov_v0.75.12_darwin_arm64.zip",
		},
		{
			Name:        "checksums.txt",
			DownloadURL: "https://github.com/k1LoW/octocov/releases/download/v0.75.12/checksums.txt",
		},
	}
	if diff := cmp.Diff(got, want, nil); diff != "" {
		t.Error(diff)
	}
}
