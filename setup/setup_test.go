package setup

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestSortPaths(t *testing.T) {
	t.Setenv("HOME", "/Users/k1low")

	tests := []struct {
		paths []string
		want  []string
	}{
		{
			[]string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/local/sbin", "/usr/bin", "/usr/sbin", "/Users/k1low/.local/bin"},
			[]string{"/Users/k1low/.local/bin", "/usr/local/bin", "/usr/bin", "/usr/local/sbin", "/usr/sbin"},
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s", tt.paths), func(t *testing.T) {
			got, err := sortPaths(tt.paths)
			if err != nil {
				t.Error(err)
			}
			if diff := cmp.Diff(got, tt.want, nil); diff != "" {
				t.Errorf("%s", diff)
			}
		})
	}
}
