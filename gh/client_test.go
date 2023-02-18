package gh

import (
	"context"
	"testing"
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
