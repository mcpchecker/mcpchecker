package tasksource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GitResolverOption configures a gitResolver.
type GitResolverOption func(*gitResolver)

// WithCacheDir sets the local directory used to cache fetched sources.
// Defaults to ~/.mcpchecker/sources.
func WithCacheDir(dir string) GitResolverOption {
	return func(r *gitResolver) {
		r.cacheDir = dir
	}
}

type gitResolver struct {
	cacheDir string
	gitBin   string
}

var fullSHAPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// NewGitResolver creates a Resolver that fetches task sources from git
// repositories and caches them locally. It shells out to the git binary,
// inheriting the user's existing credentials and configuration.
func NewGitResolver(opts ...GitResolverOption) (Resolver, error) {
	gitBin, err := exec.LookPath("git")
	if err != nil {
		return nil, fmt.Errorf("git not found in PATH: %w", err)
	}

	r := &gitResolver{
		gitBin: gitBin,
	}

	for _, opt := range opts {
		opt(r)
	}

	if r.cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to determine home directory: %w", err)
		}
		r.cacheDir = filepath.Join(home, ".mcpchecker", "sources")
	}

	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	return r, nil
}

func (r *gitResolver) Resolve(ctx context.Context, repo, ref string) (string, string, error) {
	repoURL := toRepoURL(repo)

	sha, err := r.resolveRef(ctx, repoURL, ref)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve ref %q: %w", ref, err)
	}

	cacheKey := repoCacheKey(repo)
	dir := filepath.Join(r.cacheDir, cacheKey, sha)

	// Cache hit — directory already exists.
	if info, err := os.Stat(dir); err == nil && info.IsDir() {
		return dir, sha, nil
	}

	if err := r.cloneAt(ctx, repoURL, sha, dir); err != nil {
		return "", "", fmt.Errorf("failed to fetch source: %w", err)
	}

	return dir, sha, nil
}

// resolveRef resolves a git ref (branch, tag, or SHA) to a full commit SHA.
// If ref is already a 40-character hex string it is returned directly.
// Otherwise git ls-remote is used, preferring peeled refs so annotated tags
// resolve to the underlying commit.
func (r *gitResolver) resolveRef(ctx context.Context, repoURL, ref string) (string, error) {
	if fullSHAPattern.MatchString(ref) {
		return ref, nil
	}

	out, err := r.git(ctx, "ls-remote", repoURL, ref)
	if err != nil {
		return "", err
	}

	if out == "" {
		return "", fmt.Errorf("ref %q not found in %s", ref, repoURL)
	}

	// Parse output — prefer peeled refs (^{}) for annotated tags.
	var sha, peeledSHA string
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		if strings.HasSuffix(fields[1], "^{}") {
			peeledSHA = fields[0]
		} else if sha == "" {
			sha = fields[0]
		}
	}

	if peeledSHA != "" {
		return peeledSHA, nil
	}

	if sha != "" {
		return sha, nil
	}

	return "", fmt.Errorf("could not parse SHA from ls-remote output for ref %q", ref)
}

// cloneAt fetches the tree at the given SHA into destDir. Extraction is atomic:
// content is written to a temp directory first, then renamed into place so a
// partial fetch never leaves a broken cache entry.
func (r *gitResolver) cloneAt(ctx context.Context, repoURL, sha, destDir string) error {
	parentDir := filepath.Dir(destDir)
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	tmpDir, err := os.MkdirTemp(parentDir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir) // no-op after successful rename

	if _, err := r.git(ctx, "init", tmpDir); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	if _, err := r.git(ctx, "-C", tmpDir, "fetch", "--depth", "1", repoURL, sha); err != nil {
		return fmt.Errorf("git fetch: %w", err)
	}

	if _, err := r.git(ctx, "-C", tmpDir, "checkout", "FETCH_HEAD"); err != nil {
		return fmt.Errorf("git checkout: %w", err)
	}

	// Remove .git directory — we only need the working tree.
	if err := os.RemoveAll(filepath.Join(tmpDir, ".git")); err != nil {
		return fmt.Errorf("failed to remove .git directory: %w", err)
	}

	if err := os.Rename(tmpDir, destDir); err != nil {
		return fmt.Errorf("failed to finalize cache entry: %w", err)
	}

	return nil
}

func (r *gitResolver) ContentHash(dir string) (string, error) {
	h := sha256.New()

	var paths []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}

			paths = append(paths, rel)
		}

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to walk directory: %w", err)
	}

	sort.Strings(paths)

	for _, p := range paths {
		// Include the path in the hash so renames are detected.
		fmt.Fprintf(h, "path:%s\n", p)

		data, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			return "", fmt.Errorf("failed to read file %s: %w", p, err)
		}

		h.Write(data)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// git runs a git command and returns its stdout.
func (r *gitResolver) git(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, r.gitBin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

// toRepoURL converts a repo identifier to a git-cloneable URL.
// Short-form identifiers (owner/repo) default to GitHub over HTTPS, which
// works out of the box with credential helpers, PATs, and GitHub Actions
// tokens without requiring SSH key configuration.
func toRepoURL(repo string) string {
	switch {
	case strings.HasPrefix(repo, "https://"),
		strings.HasPrefix(repo, "http://"),
		strings.HasPrefix(repo, "git@"),
		strings.HasPrefix(repo, "ssh://"),
		strings.HasPrefix(repo, "file://"),
		strings.HasPrefix(repo, "/"):
		return repo
	case strings.HasPrefix(repo, "github.com/"):
		path := strings.TrimPrefix(repo, "github.com/")
		return "https://github.com/" + path + ".git"
	default:
		return "https://github.com/" + repo + ".git"
	}
}

// repoCacheKey converts a repo identifier to a filesystem-safe cache key.
func repoCacheKey(repo string) string {
	key := repo
	key = strings.TrimPrefix(key, "https://")
	key = strings.TrimPrefix(key, "http://")
	key = strings.TrimPrefix(key, "git@")
	key = strings.TrimPrefix(key, "ssh://")
	key = strings.TrimPrefix(key, "file://")
	key = strings.TrimSuffix(key, ".git")
	key = strings.ReplaceAll(key, ":", "-")
	key = strings.ReplaceAll(key, "/", "-")
	return strings.Trim(key, "-")
}
