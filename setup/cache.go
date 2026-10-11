package setup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"golang.org/x/exp/slog"
)

const cacheSchema = 1

type Selection struct {
	Match                string
	BinMatch             string
	OS                   string
	Arch                 string
	Strict               bool
	SkipContentTypeCheck bool
}

type Verification struct {
	Checksum          string
	VerifyAttestation bool
	AttestationFlags  string
}

type CacheRequest struct {
	Host         string
	Owner        string
	Repo         string
	Tag          string
	Arch         string
	Asset        string
	Selection    Selection
	Verification Verification
}

type CacheResult struct {
	Bins   map[string]string
	BinDir string
	Hit    bool
}

type cachedBinary struct {
	Name   string
	SHA256 string
}

type cacheManifest struct {
	Schema   int
	Request  CacheRequest
	Binaries map[string]cachedBinary
}

var semverTag = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)
var repositoryComponent = regexp.MustCompile(`^[a-z0-9_.-]+$`)

func CacheVersion(tag string) (string, error) {
	if tag == "" {
		return "", errors.New("empty resolved release tag")
	}
	if validSemverTag(tag) {
		tag = strings.TrimPrefix(tag, "v")
	}
	if safeComponent(tag) {
		return tag, nil
	}
	return "tag-" + digest([]byte(tag)), nil
}

func validSemverTag(tag string) bool {
	if !semverTag.MatchString(tag) {
		return false
	}
	version, build, hasBuild := strings.Cut(strings.TrimPrefix(tag, "v"), "+")
	if hasBuild {
		for part := range strings.SplitSeq(build, ".") {
			if part == "" {
				return false
			}
		}
	}
	_, pre, hasPre := strings.Cut(version, "-")
	if hasPre {
		for part := range strings.SplitSeq(pre, ".") {
			if part == "" {
				return false
			}
			if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
				return false
			}
		}
	}
	return true
}

func safeComponent(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 128 ||
		strings.HasSuffix(value, ".") || strings.HasSuffix(value, " ") {
		return false
	}
	for _, r := range value {
		if r < 32 || r == 127 || strings.ContainsRune(`/\<>:"|?*`, r) {
			return false
		}
	}
	base, _, _ := strings.Cut(strings.ToUpper(value), ".")
	switch base {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return false
	}
	runes := []rune(base)
	if len(runes) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) &&
		strings.ContainsRune("123456789¹²³", runes[3]) {
		return false
	}
	return true
}

func CacheArch(arch string) (string, error) {
	switch arch {
	case "X86":
		return "ia32", nil
	case "X64", "ARM", "ARM64":
		return strings.ToLower(arch), nil
	default:
		return "", fmt.Errorf("unsupported RUNNER_ARCH: %q", arch)
	}
}

func RunnerTarget() (string, string, error) {
	arch, err := CacheArch(os.Getenv("RUNNER_ARCH"))
	if err != nil {
		return "", "", err
	}
	switch arch {
	case "ia32":
		arch = "386"
	case "x64":
		arch = "amd64"
	}
	switch os.Getenv("RUNNER_OS") {
	case "Linux":
		return "linux", arch, nil
	case "macOS":
		return "darwin", arch, nil
	case "Windows":
		return "windows", arch, nil
	default:
		return "", "", fmt.Errorf("unsupported RUNNER_OS: %q", os.Getenv("RUNNER_OS"))
	}
}

func ValidateActionsEnvironment() error {
	root := os.Getenv("RUNNER_TOOL_CACHE")
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\r\n") {
		return errors.New("RUNNER_TOOL_CACHE must be an absolute directory path")
	}
	if _, err := CacheArch(os.Getenv("RUNNER_ARCH")); err != nil {
		return err
	}
	if _, _, err := RunnerTarget(); err != nil {
		return err
	}
	if info, err := os.Stat(root); err == nil { // #nosec G703 -- The runner supplies the cache root, validated above.
		if !info.IsDir() {
			return errors.New("RUNNER_TOOL_CACHE is not a directory")
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := openGitHubPath()
	if err != nil {
		return err
	}
	return f.Close()
}

func openGitHubPath() (*os.File, error) {
	path := os.Getenv("GITHUB_PATH")
	if path == "" {
		return nil, errors.New("GITHUB_PATH is required in GitHub Actions")
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0) // #nosec G703 -- GITHUB_PATH is the runner-provided file-command destination.
	if err != nil {
		return nil, fmt.Errorf("open GITHUB_PATH: %w", err)
	}
	return f, nil
}

func AddPath(dir string) (err error) {
	dir, err = filepath.Abs(dir)
	if err != nil {
		return err
	}
	if strings.ContainsAny(dir, "\r\n") {
		return errors.New("installation directory contains a newline")
	}
	f, err := openGitHubPath()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	_, err = fmt.Fprintln(f, dir)
	return err
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (r CacheRequest) normalized() CacheRequest {
	r.Host = strings.ToLower(r.Host)
	r.Owner = strings.ToLower(r.Owner)
	r.Repo = strings.ToLower(r.Repo)
	return r
}

func (r CacheRequest) Path(root string) (string, error) {
	r = r.normalized()
	if !filepath.IsAbs(root) || strings.ContainsAny(root, "\r\n") {
		return "", errors.New("tool cache root must be an absolute directory path")
	}
	for _, component := range []string{r.Owner, r.Repo} {
		if !safeComponent(component) || !repositoryComponent.MatchString(component) {
			return "", fmt.Errorf("invalid repository path component: %q", component)
		}
	}
	switch r.Arch {
	case "ia32", "x64", "arm", "arm64":
	default:
		return "", fmt.Errorf("unsupported cache architecture: %q", r.Arch)
	}
	repo := r.Repo
	if r.Host != "github.com" || r.Selection != (Selection{}) {
		b, err := json.Marshal(struct {
			Schema    int
			Host      string
			Selection Selection
		}{cacheSchema, r.Host, r.Selection})
		if err != nil {
			return "", err
		}
		repo += "-" + digest(b)[:16]
	}
	version, err := CacheVersion(r.Tag)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, r.Owner, repo, version, r.Arch)
	if err := rejectSymlinks(root, path); err != nil {
		return "", err
	}
	return path, nil
}

func rejectSymlinks(root, path string) error {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fmt.Errorf("cache path is outside RUNNER_TOOL_CACHE: %s", path)
	}
	current := root
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in tool cache path: %s", current)
		}
	}
	return nil
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	_, readErr := io.Copy(h, f)
	if err := errors.Join(readErr, f.Close()); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func readCache(path string, request CacheRequest) (*CacheResult, error) {
	info, err := os.Lstat(path + ".complete")
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid completion marker: %s.complete", path)
	}
	manifestPath := filepath.Join(path, "gh-setup.json")
	info, err = os.Lstat(manifestPath)
	if errors.Is(err, fs.ErrNotExist) {
		slog.Info("Cache miss", slog.String("reason", "missing gh-setup manifest"), slog.String("path", path))
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid cache manifest file: %s", manifestPath)
	}
	b, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, err
	}
	var manifest cacheManifest
	if err := json.Unmarshal(b, &manifest); err != nil {
		return nil, fmt.Errorf("invalid tool cache manifest: %w", err)
	}
	old := manifest.Request
	if old.Host != request.Host || old.Owner != request.Owner || old.Repo != request.Repo ||
		old.Tag != request.Tag || old.Arch != request.Arch || old.Selection != request.Selection {
		return nil, fmt.Errorf("tool cache identity collision: %s", path)
	}
	if manifest.Schema != cacheSchema || old.Asset != request.Asset ||
		(request.Verification.Checksum != "" && old.Verification.Checksum != request.Verification.Checksum) ||
		(request.Verification.VerifyAttestation && (!old.Verification.VerifyAttestation ||
			old.Verification.AttestationFlags != request.Verification.AttestationFlags)) ||
		len(manifest.Binaries) == 0 {
		slog.Info("Cache miss", slog.String("reason", "asset or verification requirements changed"), slog.String("path", path))
		return nil, nil
	}
	result := &CacheResult{Bins: map[string]string{}, BinDir: filepath.Join(path, "bin"), Hit: true}
	if info, err := os.Lstat(result.BinDir); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	} else if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("invalid cached bin directory: %s", result.BinDir)
	}
	for source, binary := range manifest.Binaries {
		if !safeComponent(binary.Name) || filepath.Base(binary.Name) != binary.Name {
			return nil, fmt.Errorf("invalid cached binary name: %q", binary.Name)
		}
		bp := filepath.Join(result.BinDir, binary.Name)
		info, err := os.Lstat(bp)
		if errors.Is(err, fs.ErrNotExist) {
			slog.Info("Cache miss", slog.String("reason", "missing binary"), slog.String("path", bp))
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || (runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0) {
			return nil, nil
		}
		hash, err := fileDigest(bp)
		if err != nil {
			return nil, err
		}
		if hash != binary.SHA256 {
			slog.Info("Cache miss", slog.String("reason", "binary integrity check failed"), slog.String("path", bp))
			return nil, nil
		}
		result.Bins[source] = bp
	}
	return result, nil
}

func acquireCacheLock(ctx context.Context, path string) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("acquire tool cache lock %s: %w", path, err)
		}
		err := os.Mkdir(path, 0700)
		if err == nil {
			return nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid tool cache lock: %s", path)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("waiting for tool cache lock %s (remove abandoned locks manually): %w", path, ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func CacheBin(ctx context.Context, root string, request CacheRequest, opt *Option,
	materialize func(context.Context) (fs.FS, error)) (result *CacheResult, err error) {
	request = request.normalized()
	path, err := request.Path(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	lock := path + ".lock"
	if err := acquireCacheLock(ctx, lock); err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, os.Remove(lock)) }()
	if err := rejectSymlinks(root, path); err != nil {
		return nil, err
	}
	cached, err := readCache(path, request)
	if err != nil {
		return nil, err
	}
	if cached != nil && (opt == nil || !opt.Force) {
		return cached, nil
	}
	fsys, err := materialize(ctx)
	if err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(path), ".gh-setup-staging-")
	if err != nil {
		return nil, err
	}
	preserveStaging := false
	defer func() {
		if preserveStaging {
			err = fmt.Errorf("cache publication failed; recovery files retained at %s: %w", staging, err)
			return
		}
		err = errors.Join(err, os.RemoveAll(staging))
	}()
	binDir := filepath.Join(staging, "executables")
	if err := os.Mkdir(binDir, 0755); err != nil {
		return nil, err
	}
	stagingOpt := &Option{BinDir: binDir, BinMatch: request.Selection.BinMatch}
	bins, err := Bin(fsys, stagingOpt)
	if err != nil {
		return nil, err
	}
	if len(bins) == 0 {
		return nil, errors.New("setup failed: no binaries found in release asset")
	}
	manifest := cacheManifest{Schema: cacheSchema, Request: request, Binaries: map[string]cachedBinary{}}
	result = &CacheResult{Bins: map[string]string{}, BinDir: filepath.Join(path, "bin")}
	for source, bp := range bins {
		name := filepath.Base(bp)
		if !safeComponent(name) {
			return nil, fmt.Errorf("unsafe binary name: %q", name)
		}
		hash, err := fileDigest(bp)
		if err != nil {
			return nil, err
		}
		manifest.Binaries[source] = cachedBinary{Name: name, SHA256: hash}
		result.Bins[source] = filepath.Join(result.BinDir, name)
	}
	b, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(staging, "gh-setup.json"), b, 0600); err != nil {
		return nil, err
	}
	if err := publishCache(path, staging); err != nil {
		preserveStaging = true
		return nil, err
	}
	return result, nil
}

func publishCache(path, staging string) (err error) {
	backup := filepath.Join(staging, "previous")
	markerBackup := filepath.Join(staging, "previous.complete")
	hadDirectory, hadMarker, published := false, false, false
	if _, err := os.Lstat(path); err == nil {
		hadDirectory = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := os.Lstat(path + ".complete"); err == nil {
		hadMarker = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if hadMarker {
		if err := os.Rename(path+".complete", markerBackup); err != nil {
			return err
		}
	}
	defer func() {
		if err == nil {
			return
		}
		if published {
			err = errors.Join(err, os.RemoveAll(path))
			if removeErr := os.Remove(path + ".complete"); removeErr != nil && !errors.Is(removeErr, fs.ErrNotExist) {
				err = errors.Join(err, removeErr)
			}
		}
		if _, statErr := os.Lstat(backup); statErr == nil {
			err = errors.Join(err, os.Rename(backup, path))
		} else if !errors.Is(statErr, fs.ErrNotExist) {
			err = errors.Join(err, statErr)
		}
		if hadMarker {
			err = errors.Join(err, os.Rename(markerBackup, path+".complete"))
		}
	}()
	if hadDirectory {
		if err := os.Rename(path, backup); err != nil {
			return err
		}
	}
	next := filepath.Join(staging, "next")
	if err := os.Mkdir(next, 0755); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(staging, "executables"), filepath.Join(next, "bin")); err != nil {
		return err
	}
	if err := os.Rename(filepath.Join(staging, "gh-setup.json"), filepath.Join(next, "gh-setup.json")); err != nil {
		return err
	}
	if err := os.Rename(next, path); err != nil {
		return err
	}
	published = true
	return os.WriteFile(path+".complete", nil, 0600)
}
