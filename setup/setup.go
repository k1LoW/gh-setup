package setup

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/h2non/filetype"
)

type SetupOption struct {
	BinDir   string
	BinMatch string
	Force    bool
}

func Bin(fsys fs.FS, opt *SetupOption) (map[string]string, error) {
	var (
		bd    string
		bm    *regexp.Regexp
		force bool
		err   error
	)
	m := map[string]string{}
	if opt != nil {
		force = opt.Force
		bd = opt.BinDir
		if opt.BinMatch != "" {
			bm, err = regexp.Compile(opt.BinMatch)
			if err != nil {
				return nil, err
			}
		}
	}
	if bd == "" {
		bd, err = binDir()
		if err != nil {
			return nil, err
		}
	}
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		log.Println("extract file:", path)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if bm != nil {
			if !bm.MatchString(path) {
				return nil
			}
		} else {
			for _, i := range ignoreBinnameKeywords {
				if strings.Contains(filepath.ToSlash(strings.ToLower(path)), filepath.ToSlash(strings.ToLower(i))) {
					return nil
				}
			}
		}

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		if isBinary(b) {
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
var ignorePathKeywords = []string{
	"homebrew",
	"X11",
	"/usr/local/opt",
	"sbin",
	"perl",
	"git",
	"/go/",
	".asdf",
	".cargo",
	".dotnet",
	".ghcup",
	".yarn",
	"/Library/",
	"hostedtoolcache",
}
var ignoreBinnameKeywords = []string{
	"CHANGELOG",
	"README",
	"CREDIT",
	"LICENSE",
}

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
		for _, i := range ignorePathKeywords {
			if strings.Contains(filepath.ToSlash(strings.ToLower(p)), filepath.ToSlash(strings.ToLower(i))) {
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

func isBinary(b []byte) bool {
	// FIXME: On Windows, it can't be detected at all.
	const binaryContentType = "application/octet-stream"
	contentType := http.DetectContentType(b)
	log.Println("content type:", contentType)
	if contentType == binaryContentType {
		return true
	}
	typ, err := filetype.Match(b)
	if err != nil {
		return false
	}
	log.Printf("file type: %v\n", typ)
	if typ == filetype.Unknown {
		return true
	}
	return false
}
