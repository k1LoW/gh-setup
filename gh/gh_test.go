package gh

import (
	"context"
	"io/fs"
	"os"
	"testing"

	"github.com/k1LoW/go-github-client/v52/factory"
)

func TestMain(m *testing.M) {
	os.Setenv("GH_CONFIG_DIR", "/tmp") // Disable reading credentials from config
	m.Run()
}

func TestGetReleaseAsset(t *testing.T) {
	tests := []struct {
		owner    string
		repo     string
		opt      *AssetOption
		wantFile string
		useToken bool
	}{
		{"k1LoW", "tbls", nil, "tbls", true},
		{"k1LoW", "tbls", nil, "tbls", false},
		{"k1LoW", "tbls", &AssetOption{Version: "v1.60.0"}, "tbls", true},
		{"k1LoW", "tbls", &AssetOption{Version: "v1.60.0"}, "tbls", false},
	}
	ctx := context.Background()
	token, _, _, _ := factory.GetTokenAndEndpoints()
	for _, tt := range tests {
		if tt.useToken {
			t.Setenv("GITHUB_TOKEN", token)
		} else {
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("GH_TOKEN", "")
		}
		_, fsys, err := GetReleaseAsset(ctx, tt.owner, tt.repo, tt.opt)
		if err != nil {
			t.Error(err)
			continue
		}
		if _, err := fs.ReadFile(fsys, tt.wantFile); err != nil {
			t.Error(err)
		}
	}
}
