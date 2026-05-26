package tasksource

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// testGit runs a git command in dir and returns stdout. It sets dummy author
// info so commits work in environments without global git config.
func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, string(out))
	return strings.TrimSpace(string(out))
}

// setupTestRepo creates a local bare repo containing the given files on a
// "main" branch and returns the bare repo path and the commit SHA.
func setupTestRepo(t *testing.T, files map[string]string) (bareDir, commitSHA string) {
	t.Helper()

	bareDir = t.TempDir()
	testGit(t, bareDir, "init", "--bare")

	workDir := t.TempDir()
	testGit(t, workDir, "init")

	for name, content := range files {
		path := filepath.Join(workDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}

	testGit(t, workDir, "add", "-A")
	testGit(t, workDir, "commit", "-m", "initial commit")
	testGit(t, workDir, "push", bareDir, "HEAD:refs/heads/main")

	sha := testGit(t, workDir, "rev-parse", "HEAD")
	return bareDir, sha
}

func TestToRepoURL(t *testing.T) {
	tt := map[string]struct {
		input    string
		expected string
	}{
		"short form": {
			input:    "owner/repo",
			expected: "https://github.com/owner/repo.git",
		},
		"github.com prefix": {
			input:    "github.com/owner/repo",
			expected: "https://github.com/owner/repo.git",
		},
		"https url": {
			input:    "https://github.com/owner/repo.git",
			expected: "https://github.com/owner/repo.git",
		},
		"ssh url": {
			input:    "git@github.com:owner/repo.git",
			expected: "git@github.com:owner/repo.git",
		},
		"local path": {
			input:    "/tmp/bare-repo",
			expected: "/tmp/bare-repo",
		},
		"file url": {
			input:    "file:///tmp/bare-repo",
			expected: "file:///tmp/bare-repo",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expected, toRepoURL(tc.input))
		})
	}
}

func TestRepoCacheKey(t *testing.T) {
	tt := map[string]struct {
		input    string
		expected string
	}{
		"short form": {
			input:    "owner/repo",
			expected: "owner-repo",
		},
		"github.com prefix": {
			input:    "github.com/owner/repo",
			expected: "github.com-owner-repo",
		},
		"https url": {
			input:    "https://github.com/owner/repo.git",
			expected: "github.com-owner-repo",
		},
		"ssh url": {
			input:    "git@github.com:owner/repo.git",
			expected: "github.com-owner-repo",
		},
		"local path": {
			input:    "/tmp/bare-repo",
			expected: "tmp-bare-repo",
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expected, repoCacheKey(tc.input))
		})
	}
}

func TestResolve(t *testing.T) {
	requireGit(t)

	files := map[string]string{
		"task.yaml":        "kind: Task\n",
		"nested/other.yaml": "kind: Task\n",
	}
	bareDir, sha := setupTestRepo(t, files)
	cacheDir := t.TempDir()

	resolver, err := NewGitResolver(WithCacheDir(cacheDir))
	require.NoError(t, err)

	ctx := context.Background()

	// Resolve by branch name — cache miss, should fetch.
	dir, commit, err := resolver.Resolve(ctx, bareDir, "main")
	require.NoError(t, err)
	assert.Equal(t, sha, commit)
	assert.DirExists(t, dir)
	assert.FileExists(t, filepath.Join(dir, "task.yaml"))
	assert.FileExists(t, filepath.Join(dir, "nested/other.yaml"))

	// .git directory should not be present in the cached copy.
	assert.NoDirExists(t, filepath.Join(dir, ".git"))
}

func TestResolve_BySHA(t *testing.T) {
	requireGit(t)

	bareDir, sha := setupTestRepo(t, map[string]string{"a.txt": "hello"})
	cacheDir := t.TempDir()

	resolver, err := NewGitResolver(WithCacheDir(cacheDir))
	require.NoError(t, err)

	ctx := context.Background()

	dir, commit, err := resolver.Resolve(ctx, bareDir, sha)
	require.NoError(t, err)
	assert.Equal(t, sha, commit)

	data, err := os.ReadFile(filepath.Join(dir, "a.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestResolve_CacheHit(t *testing.T) {
	requireGit(t)

	bareDir, sha := setupTestRepo(t, map[string]string{"f.txt": "data"})
	cacheDir := t.TempDir()

	resolver, err := NewGitResolver(WithCacheDir(cacheDir))
	require.NoError(t, err)

	ctx := context.Background()

	dir1, _, err := resolver.Resolve(ctx, bareDir, "main")
	require.NoError(t, err)

	// Delete the bare repo so any network call would fail.
	require.NoError(t, os.RemoveAll(bareDir))

	// Resolve again using the SHA — ls-remote is skipped and the cache
	// directory already exists, so this must succeed without network access.
	dir2, commit2, err := resolver.Resolve(ctx, bareDir, sha)
	require.NoError(t, err)
	assert.Equal(t, dir1, dir2)
	assert.Equal(t, sha, commit2)
}

func TestResolve_InvalidRef(t *testing.T) {
	requireGit(t)

	bareDir, _ := setupTestRepo(t, map[string]string{"f.txt": "x"})
	cacheDir := t.TempDir()

	resolver, err := NewGitResolver(WithCacheDir(cacheDir))
	require.NoError(t, err)

	_, _, err = resolver.Resolve(context.Background(), bareDir, "nonexistent-branch")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolve_InvalidRepo(t *testing.T) {
	requireGit(t)

	cacheDir := t.TempDir()
	resolver, err := NewGitResolver(WithCacheDir(cacheDir))
	require.NoError(t, err)

	_, _, err = resolver.Resolve(context.Background(), "/nonexistent/repo/path", "main")
	require.Error(t, err)
}

func TestContentHash(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub/b.txt"), []byte("world"), 0o644))

	resolver, err := NewGitResolver(WithCacheDir(t.TempDir()))
	require.NoError(t, err)

	hash1, err := resolver.ContentHash(dir)
	require.NoError(t, err)
	assert.Len(t, hash1, 64) // SHA-256 hex

	// Same content produces the same hash.
	hash2, err := resolver.ContentHash(dir)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Changing file content changes the hash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("changed"), 0o644))
	hash3, err := resolver.ContentHash(dir)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestContentHash_RenameChangesHash(t *testing.T) {
	requireGit(t)

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("same"), 0o644))

	resolver, err := NewGitResolver(WithCacheDir(t.TempDir()))
	require.NoError(t, err)

	hash1, err := resolver.ContentHash(dir)
	require.NoError(t, err)

	// Rename the file — same content but different path should change the hash.
	require.NoError(t, os.Rename(filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")))

	hash2, err := resolver.ContentHash(dir)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestContentHash_EmptyDir(t *testing.T) {
	requireGit(t)

	resolver, err := NewGitResolver(WithCacheDir(t.TempDir()))
	require.NoError(t, err)

	hash, err := resolver.ContentHash(t.TempDir())
	require.NoError(t, err)
	assert.Len(t, hash, 64)
}

func TestContentHash_NonexistentDir(t *testing.T) {
	requireGit(t)

	resolver, err := NewGitResolver(WithCacheDir(t.TempDir()))
	require.NoError(t, err)

	_, err = resolver.ContentHash("/nonexistent/dir")
	require.Error(t, err)
}
