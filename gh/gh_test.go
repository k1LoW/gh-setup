package gh

import (
	"context"
	"io/fs"
	"testing"
)

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
	}
	ctx := context.Background()
	for _, tt := range tests {
		if !tt.useToken {
			t.Setenv("GITHUB_TOKEN", "")
			t.Setenv("GH_TOKEN", "")
		}
		_, fsys, err := GetReleaseAsset(ctx, tt.owner, tt.repo, tt.opt)
		if err != nil {
			t.Error(err)
			return
		}
		if _, err := fs.ReadFile(fsys, tt.wantFile); err != nil {
			t.Error(err)
			return
		}
	}
}
