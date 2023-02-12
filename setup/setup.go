package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func Bin(fsys fs.FS, bd string, force bool) (map[string]string, error) {
	const binaryContentType = "application/octet-stream"
	var err error
	m := map[string]string{}
	if bd == "" {
		bd, err = binDir()
		if err != nil {
			return nil, err
		}
	}
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		contentType := http.DetectContentType(b)
		if contentType == binaryContentType {
			perm := "0755"
			perm32, err := strconv.ParseUint(perm, 8, 32)
			if err != nil {
				return err
			}
			bp := filepath.Join(bd, filepath.Base(path))
			if _, err := os.Stat(bp); err == nil && !force {
				return fmt.Errorf("%s already exist", bp)
			}
			if err := os.WriteFile(bp, b, os.FileMode(perm32)); err != nil {
				return err
			}
			m[path] = bp
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return m, nil
}

var priorityPaths = []string{"/usr/local/bin", "/usr/bin"}
var ignoreKeywords = []string{"homebrew", "asdf", "X11", "/usr/local/opt", "sbin", "perl", "git", "go/bin"}

func binDir() (string, error) {
	if os.Getenv("PATH") == "" {
		return "", errors.New("env PATH not set")
	}
	paths, err := sortPaths(filepath.SplitList(os.Getenv("PATH")))
	if err != nil {
		return "", err
	}
	for _, p := range paths {
		f := filepath.Join(p, "gh-setup-tmp")
		if err := os.WriteFile(f, []byte("test"), os.ModePerm); err == nil {
			if err := os.Remove(f); err != nil {
				return "", err
			}
			return p, nil
		}
	}
	return "", fmt.Errorf("could not find a writable bin path: %s", strings.Join(paths, string(filepath.ListSeparator)))
}

func sortPaths(paths []string) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	filtered := []string{}
L:
	for _, p := range paths {
		for _, i := range ignoreKeywords {
			if strings.Contains(strings.ToLower(p), strings.ToLower(i)) {
				continue L
			}
		}
		filtered = append(filtered, p)
	}
	sort.Slice(filtered, func(i, j int) bool {
		pi := filtered[i]
		pj := filtered[j]
		switch {
		case strings.HasPrefix(pi, home) && !strings.HasPrefix(pj, home):
			return true
		case !strings.HasPrefix(pi, home) && strings.HasPrefix(pj, home):
			return false
		case strings.HasPrefix(pi, home) && strings.HasPrefix(pj, home):
			return pi < pj
		case hasPrefixes(pi, priorityPaths) >= 0 && hasPrefixes(pj, priorityPaths) < 0:
			return true
		case hasPrefixes(pi, priorityPaths) < 0 && hasPrefixes(pj, priorityPaths) >= 0:
			return false
		case hasPrefixes(pi, priorityPaths) >= 0 && hasPrefixes(pj, priorityPaths) >= 0:
			return hasPrefixes(pi, priorityPaths) < hasPrefixes(pj, priorityPaths)
		}
		return false
	})

	return filtered, nil
}

func hasPrefixes(in string, ps []string) int {
	for i, p := range ps {
		if strings.HasPrefix(in, p) {
			return i
		}
	}
	return -1
}
