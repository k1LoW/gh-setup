package setup

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/h2non/filetype"
	"golang.org/x/exp/slog"
)

type Option struct {
	BinDir   string
	BinMatch string
	Checksum string
	Force    bool
}

func Bin(fsys fs.FS, opt *Option) (map[string]string, error) {
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
		slog.Info("Extract target", slog.String("path", path))
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if bm != nil {
			if !bm.MatchString(path) {
				slog.Info("Skip", slog.String("Reason", "No match for --bin-match"), slog.String("path", path), slog.String("match", bm.String()))
				return nil
			}
		} else {
			for _, i := range ignoreBinnameKeywords {
				if strings.Contains(filepath.ToSlash(strings.ToLower(path)), filepath.ToSlash(strings.ToLower(i))) {
					slog.Info("Skip", slog.String("Reason", "Matched the ignore filename keywords"), slog.String("path", path), slog.String("list", fmt.Sprintf("%v", ignoreBinnameKeywords)))
					return nil
				}
			}
		}

		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}

		if err := checksum(b, opt.Checksum); err != nil {
			return err
		}

		if !isBinary(b) {
			slog.Info("Skip", slog.String("Reason", "Not determined to be a binary file"), slog.String("path", path))
			return nil
		}

		slog.Info("Determine as a binary file", slog.String("path", path))
		perm := "0755"
		perm32, err := strconv.ParseUint(perm, 8, 32)
		if err != nil {
			return err
		}
		bp := filepath.Join(bd, filepath.Base(path))
		slog.Info("Write file", slog.String("bin path", bp))
		if _, err := os.Stat(bp); err == nil && !force {
			return fmt.Errorf("%s already exist", bp)
		}
		if err := os.WriteFile(bp, b, os.FileMode(perm32)); err != nil {
			return err
		}
		m[path] = bp
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
	slog.Info("Detect content type", slog.String("content type", contentType))
	if contentType == binaryContentType {
		return true
	}
	typ, err := filetype.Match(b)
	if err != nil {
		return false
	}
	slog.Info("Detect file type", slog.String("file type", fmt.Sprintf("%v", typ)))
	return typ == filetype.Unknown
}

func checksum(b []byte, c string) error {
	if c == "" {
		return nil // No checksum verification needed
	}

	var (
		alg  string
		want string
	)

	// Check if the format is "algorithm:hash"
	parts := strings.SplitN(c, ":", 2)
	if len(parts) == 2 {
		alg = strings.ToLower(parts[0])
		want = strings.ToLower(parts[1])
	} else {
		// If no alg is specified, try to determine it based on the length of the checksum
		want = strings.ToLower(c)
		// Try to match based on length and value
		switch len(want) {
		case 8: // CRC32
			alg = "crc32"
		case 32: // MD5
			alg = "md5"
		case 40: // SHA-1
			alg = "sha1"
		case 64: // SHA-256
			alg = "sha256"
		case 128: // SHA-512
			alg = "sha512"
		}
	}

	var got string
	switch alg {
	case "crc32":
		got = fmt.Sprintf("%08x", crc32.ChecksumIEEE(b))
	case "md5":
		sum := md5.Sum(b)
		got = hex.EncodeToString(sum[:])
	case "sha1":
		sum := sha1.Sum(b)
		got = hex.EncodeToString(sum[:])
	case "sha256":
		sum := sha256.Sum256(b)
		got = hex.EncodeToString(sum[:])
	case "sha512":
		sum := sha512.Sum512(b)
		got = hex.EncodeToString(sum[:])
	default:
		return fmt.Errorf("unsupported alg: %s", alg)
	}

	if got != want {
		return fmt.Errorf("checksum mismatch: expected=%s, calculated=%s", want, got)
	}
	return nil
}
