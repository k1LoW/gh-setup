package gh

import (
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"crypto/md5"  //nolint:gosec
	"crypto/sha1" //nolint:gosec
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"net/http"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing/fstest"
	"time"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/nlepage/go-tarfs"
	"golang.org/x/exp/slog"
)

var osDict = map[string][]string{
	"darwin":  {"darwin", "macos"},
	"windows": {"windows"},
	"linux":   {"linux"},
}

var archDict = map[string][]string{
	"amd64": {"amd64", "x86_64", "x64"},
	"arm64": {"arm64", "aarch64"},
}

var supportContentType = []string{
	// zip
	"application/zip",
	"application/x-zip-compressed",
	// tar.gz
	"application/gzip",
	"application/x-gtar",
	"application/x-gzip",
	// tar.bz2
	"application/x-bzip2",
	// binary
	"application/octet-stream",
}

const versionLatest = "latest"

type AssetOption struct {
	Match                string
	Version              string
	OS                   string
	Arch                 string
	Strict               bool
	SkipContentTypeCheck bool
	Checksum             string
}

func GetReleaseAsset(ctx context.Context, owner, repo string, opt *AssetOption) (*releaseAsset, fs.FS, error) {
	c, err := newClient(ctx, owner, repo)
	if err != nil {
		return nil, nil, err
	}
	assets, err := c.getReleaseAssets(ctx, opt)
	if err != nil {
		return nil, nil, err
	}
	a, err := detectAsset(assets, opt)
	if err != nil {
		return nil, nil, err
	}
	b, err := c.downloadAsset(ctx, a)
	if err != nil {
		return nil, nil, err
	}

	if opt != nil {
		if err := checksum(b, opt.Checksum); err != nil {
			return nil, nil, err
		}
	}

	fsys, err := makeFS(ctx, b, repo, a.Name, []string{a.ContentType, http.DetectContentType(b)})
	if err != nil {
		return nil, nil, err
	}
	return a, fsys, nil
}

func DetectHostOwnerRepo(ownerrepo string) (string, string, string, error) {
	var host, owner, repo string
	if ownerrepo == "" {
		r, err := repository.Current()
		if err != nil {
			return "", "", "", err
		}
		host = r.Host
		owner = r.Owner
		repo = r.Name
	} else {
		r, err := repository.Parse(ownerrepo)
		if err != nil {
			return "", "", "", err
		}
		host = r.Host
		owner = r.Owner
		repo = r.Name
	}
	return host, owner, repo, nil
}

func detectAsset(assets []*releaseAsset, opt *AssetOption) (*releaseAsset, error) {
	slog.Info("Detect the most appropriate asset from all assets")
	var (
		od, ad, om *regexp.Regexp
		err        error
	)
	if opt != nil && opt.Match != "" {
		om, err = regexp.Compile(opt.Match)
		if err != nil {
			return nil, err
		}
	}
	if opt != nil && opt.OS != "" {
		od = getDictRegexp(opt.OS, osDict)
	} else {
		od = getDictRegexp(runtime.GOOS, osDict)
	}
	if opt != nil && opt.Arch != "" {
		ad = getDictRegexp(opt.Arch, archDict)
	} else {
		ad = getDictRegexp(runtime.GOARCH, archDict)
	}

	type assetScore struct {
		asset *releaseAsset
		score int
	}
	assetScores := []*assetScore{}
	for _, a := range assets {
		if (opt == nil || !opt.SkipContentTypeCheck) && a.ContentType != "" && !contains(supportContentType, a.ContentType) {
			slog.Info("Skip",
				slog.String("name", a.Name),
				slog.String("reason", "Unsupported content type"),
				slog.String("content type", a.ContentType),
				slog.String("support content type", fmt.Sprintf("%v", supportContentType)))
			continue
		}
		as := &assetScore{
			asset: a,
			score: 0,
		}
		if om != nil && om.MatchString(a.Name) {
			slog.Info("it matched --match", slog.String("name", a.Name), slog.String("match", om.String()))
			as.score += 13
		}
		assetScores = append(assetScores, as)
		// os
		if od.MatchString(a.Name) {
			as.score += 7
		}
		// arch
		if ad.MatchString(a.Name) {
			as.score += 3
		}
		// content type
		if a.ContentType == "application/octet-stream" {
			as.score += 1
		}
		if opt != nil && opt.Strict && om != nil {
			slog.Info("Set score", slog.String("name", a.Name), slog.Int("score", as.score))
		}
	}
	if opt != nil && opt.Strict && om != nil {
		return nil, fmt.Errorf("no matching assets found: %s", opt.Match)
	}
	if len(assetScores) == 0 {
		return nil, errors.New("no matching assets found")
	}

	sort.Slice(assetScores, func(i, j int) bool {
		return assetScores[i].score > assetScores[j].score
	})

	if opt != nil && opt.Strict && assetScores[0].score < 10 {
		return nil, fmt.Errorf("no matching assets found for OS/Arch: %s/%s", opt.OS, opt.Arch)
	}
	slog.Info("Select the one with the highest score", slog.String("name", assetScores[0].asset.Name), slog.Int("score", assetScores[0].score))
	return assetScores[0].asset, nil
}

func getDictRegexp(key string, dict map[string][]string) *regexp.Regexp {
	for k, d := range dict {
		if strings.ToLower(key) == k {
			return regexp.MustCompile(fmt.Sprintf("(?i)(%s)", strings.Join(d, "|")))
		}
	}
	return regexp.MustCompile(fmt.Sprintf("(?i)(%s)", strings.ToLower(key)))
}

func contains(s []string, e string) bool {
	for _, v := range s {
		if e == v {
			return true
		}
	}
	return false
}

func makeFS(ctx context.Context, b []byte, repo, name string, contentTypes []string) (fs.FS, error) {
	switch {
	case matchContentTypes([]string{"application/zip", "application/x-zip-compressed"}, contentTypes):
		return zip.NewReader(bytes.NewReader(b), int64(len(b)))
	case matchContentTypes([]string{"application/gzip", "application/x-gzip"}, contentTypes):
		gr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		if strings.HasSuffix(name, ".tar.gz") {
			fsys, err := tarfs.New(gr)
			if err != nil {
				return nil, err
			}
			return fsys, nil
		}
		b, err := io.ReadAll(gr)
		if err != nil {
			return nil, err
		}
		fsys := fstest.MapFS{}
		fsys[repo] = &fstest.MapFile{
			Data:    b,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
		}
		return fsys, nil
	case matchContentTypes([]string{"application/x-bzip2"}, contentTypes):
		br := bzip2.NewReader(bytes.NewReader(b))
		if strings.HasSuffix(name, ".tar.bz2") {
			fsys, err := tarfs.New(br)
			if err != nil {
				return nil, err
			}
			return fsys, nil
		}
		b, err := io.ReadAll(br)
		if err != nil {
			return nil, err
		}
		fsys := fstest.MapFS{}
		fsys[repo] = &fstest.MapFile{
			Data:    b,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
		}
		return fsys, nil
	case matchContentTypes([]string{"application/octet-stream"}, contentTypes):
		fsys := fstest.MapFS{}
		fsys[repo] = &fstest.MapFile{
			Data:    b,
			Mode:    fs.ModePerm,
			ModTime: time.Now(),
		}
		return fsys, nil
	default:
		return nil, fmt.Errorf("unsupport content types: %s", contentTypes)
	}
}

func matchContentTypes(m, ct []string) bool {
	for _, v := range m {
		for _, vv := range ct {
			if v == vv {
				return true
			}
		}
	}
	return false
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
		sum := md5.Sum(b) //nolint:gosec
		got = hex.EncodeToString(sum[:])
	case "sha1":
		sum := sha1.Sum(b) //nolint:gosec
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
