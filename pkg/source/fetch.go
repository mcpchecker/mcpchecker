package source

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/mcpchecker/mcpchecker/pkg/task"
	"sigs.k8s.io/yaml"
)

// Fetcher handles fetching of external source repositories.
type Fetcher interface {
	// ResolveRef resolves a git ref (branch/tag/SHA) to a full commit SHA.
	// An empty ref resolves HEAD.
	ResolveRef(ctx context.Context, repo, ref string) (string, error)

	// Fetch downloads the repository at the given commit SHA into destDir.
	// Returns the sha256 hash of the downloaded content as "sha256:<hex>".
	Fetch(ctx context.Context, repo, commit, destDir string) (hash string, err error)
}

// GitHubFetcher implements Fetcher using the GitHub archive API.
// It does not require git to be installed.
type GitHubFetcher struct{}

var _ Fetcher = &GitHubFetcher{}

// ResolveRef resolves a branch, tag, or SHA ref to a full commit SHA via the GitHub API.
func (f *GitHubFetcher) ResolveRef(ctx context.Context, repo, ref string) (string, error) {
	owner, repoName, err := ParseGitHubRepo(repo)
	if err != nil {
		return "", err
	}

	if ref == "" {
		ref = "HEAD"
	}
	if isCommitSHA(ref) {
		return ref, nil
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", owner, repoName, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to query GitHub API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d for %s@%s", resp.StatusCode, repo, ref)
	}

	var result struct {
		SHA string `json:"sha"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode GitHub API response: %w", err)
	}
	if result.SHA == "" {
		return "", fmt.Errorf("GitHub API returned empty SHA for %s@%s", repo, ref)
	}

	return result.SHA, nil
}

// Fetch downloads the repository tarball at commit and extracts it to destDir.
// Returns the sha256 hash of the tarball as "sha256:<hex>".
func (f *GitHubFetcher) Fetch(ctx context.Context, repo, commit, destDir string) (string, error) {
	owner, repoName, err := ParseGitHubRepo(repo)
	if err != nil {
		return "", err
	}

	if err := os.RemoveAll(destDir); err != nil {
		return "", fmt.Errorf("failed to clean dest dir: %w", err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create dest dir: %w", err)
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/tarball/%s", owner, repoName, commit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download tarball for %s@%s: %w", repo, commit, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub returned %d downloading tarball for %s@%s", resp.StatusCode, repo, commit)
	}

	h := sha256.New()
	reader := io.TeeReader(resp.Body, h)
	if err := extractTarGz(reader, destDir); err != nil {
		return "", fmt.Errorf("failed to extract tarball for %s@%s: %w", repo, commit, err)
	}

	hash := "sha256:" + hex.EncodeToString(h.Sum(nil))
	return hash, nil
}

// extractTarGz extracts a gzipped tar stream into destDir, stripping the first path component.
// The GitHub tarball top-level directory (e.g. "owner-repo-abc123/") is stripped.
// Entries that would escape destDir are rejected.
func extractTarGz(r io.Reader, destDir string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	destDir = filepath.Clean(destDir)
	tr := tar.NewReader(gr)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar entry: %w", err)
		}

		// Strip the top-level directory (e.g. "owner-repo-abc123/")
		parts := strings.SplitN(hdr.Name, "/", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		relPath := parts[1]

		destPath := filepath.Join(destDir, filepath.FromSlash(relPath))
		// Reject path traversal
		if !strings.HasPrefix(destPath+string(filepath.Separator), destDir+string(filepath.Separator)) {
			return fmt.Errorf("tar entry %q would escape destination directory", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0777)
			if err != nil {
				return fmt.Errorf("failed to create %s: %w", destPath, err)
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return fmt.Errorf("failed to write %s: %w", destPath, err)
			}
			f.Close()
		case tar.TypeSymlink:
			// Validate symlink target stays within destDir
			linkTarget := filepath.Join(filepath.Dir(destPath), hdr.Linkname)
			if !strings.HasPrefix(linkTarget+string(filepath.Separator), destDir+string(filepath.Separator)) {
				return fmt.Errorf("symlink %q → %q would escape destination directory", hdr.Name, hdr.Linkname)
			}
			if err := os.Symlink(hdr.Linkname, destPath); err != nil && !os.IsExist(err) {
				return err
			}
		}
	}

	return nil
}

// ParseGitHubRepo parses "github.com/owner/repo" (with or without https:// prefix)
// into owner and repo name.
func ParseGitHubRepo(repo string) (owner, repoName string, err error) {
	repo = strings.TrimPrefix(repo, "https://")
	repo = strings.TrimPrefix(repo, "http://")

	after, ok := strings.CutPrefix(repo, "github.com/")
	if !ok {
		return "", "", fmt.Errorf("unsupported repo %q: must start with github.com/", repo)
	}

	parts := strings.SplitN(after, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub repo %q: expected github.com/owner/repo", repo)
	}

	return parts[0], parts[1], nil
}

// SourceCacheDir returns the local cache directory for a source at a specific commit.
// Layout: ~/.mcpchecker/sources/<owner>-<repo>/<commit>/
func SourceCacheDir(repo, commit string) (string, error) {
	owner, repoName, err := ParseGitHubRepo(repo)
	if err != nil {
		return "", err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine home directory: %w", err)
	}

	return filepath.Join(home, ".mcpchecker", "sources", owner+"-"+repoName, commit), nil
}

// VerifyHash checks that the cached content hash file matches expectedHash.
// Returns nil if no hash file is stored (first-time access).
func VerifyHash(cacheDir, expectedHash string) error {
	stored, err := readHashFile(cacheDir)
	if err != nil {
		return err
	}
	if stored == "" {
		return nil // no stored hash, trust cache
	}
	if stored != expectedHash {
		return fmt.Errorf("cache hash mismatch: stored %s, expected %s (cache may be corrupted; delete %s to re-fetch)", stored, expectedHash, cacheDir)
	}
	return nil
}

// WriteHash writes a content hash to the cache directory for future verification.
func WriteHash(cacheDir, hash string) error {
	return os.WriteFile(filepath.Join(cacheDir, ".hash"), []byte(hash+"\n"), 0644)
}

func readHashFile(cacheDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(cacheDir, ".hash"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// ScanTaskRequirements walks dir recursively and collects mcpServer names from task requires.
// Returns a map of server name → count of tasks requiring it.
func ScanTaskRequirements(dir string) (map[string]int, error) {
	servers := make(map[string]int)

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !isYAMLFile(path) {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if !strings.Contains(string(data), "kind: Task") {
			return nil
		}

		tc, parseErr := task.Read(data, filepath.Dir(path))
		if parseErr != nil {
			return nil
		}
		if tc.Spec == nil {
			return nil
		}

		for _, req := range tc.Spec.Requires {
			if req.McpServer != nil && *req.McpServer != "" {
				servers[*req.McpServer]++
			}
		}
		return nil
	})

	return servers, err
}

// UpdateSourceServerMapping reads the eval YAML at evalFilePath, sets the serverMapping
// for the named source, and writes the file back. Comments are not preserved.
func UpdateSourceServerMapping(evalFilePath, sourceName string, mapping map[string]string) error {
	data, err := os.ReadFile(evalFilePath)
	if err != nil {
		return fmt.Errorf("failed to read eval file: %w", err)
	}

	var doc map[string]any
	if err := unmarshalYAML(data, &doc); err != nil {
		return fmt.Errorf("failed to parse eval file: %w", err)
	}

	config := ensureMap(doc, "config")
	sources := ensureMap(config, "sources")
	src := ensureMap(sources, sourceName)

	mappingAny := make(map[string]any, len(mapping))
	for k, v := range mapping {
		mappingAny[k] = v
	}
	src["serverMapping"] = mappingAny

	out, err := marshalYAML(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal eval file: %w", err)
	}

	return os.WriteFile(evalFilePath, out, 0644)
}

// DirExists returns true if path exists and is a directory.
func DirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func ensureMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if sub, ok := v.(map[string]any); ok {
			return sub
		}
	}
	sub := make(map[string]any)
	m[key] = sub
	return sub
}

func isYAMLFile(path string) bool {
	ext := filepath.Ext(path)
	return ext == ".yaml" || ext == ".yml"
}

func isCommitSHA(s string) bool {
	if len(s) != 40 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

func unmarshalYAML(data []byte, v any) error {
	return yaml.Unmarshal(data, v)
}

func marshalYAML(v any) ([]byte, error) {
	return yaml.Marshal(v)
}
