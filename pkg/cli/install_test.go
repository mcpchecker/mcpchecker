package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/source"
	"sigs.k8s.io/yaml"
)

// mockFetcher implements source.Fetcher for tests.
type mockFetcher struct {
	// resolveCommit is the commit SHA returned by ResolveRef.
	resolveCommit string
	// fetchHash is the content hash returned by Fetch.
	fetchHash string
	// fetchFiles maps relative file path → content written into destDir on Fetch.
	fetchFiles map[string]string
	// resolveErr, if set, is returned by ResolveRef.
	resolveErr error
}

var _ source.Fetcher = &mockFetcher{}

func (m *mockFetcher) ResolveRef(_ context.Context, _, _ string) (string, error) {
	if m.resolveErr != nil {
		return "", m.resolveErr
	}
	return m.resolveCommit, nil
}

func (m *mockFetcher) Fetch(_ context.Context, _, _, destDir string) (string, error) {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", err
	}
	for rel, content := range m.fetchFiles {
		path := filepath.Join(destDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return "", err
		}
	}
	hash := m.fetchHash
	if hash == "" {
		hash = "sha256:aabbcc"
	}
	return hash, nil
}

// makeEvalFile writes an eval.yaml to dir with the given sources and mcpConfigFile.
func makeEvalFile(t *testing.T, dir string, sources map[string]map[string]any, mcpConfigFile string) string {
	t.Helper()

	doc := map[string]any{
		"kind":       "Eval",
		"apiVersion": "mcpchecker/v1alpha1",
		"metadata":   map[string]any{"name": "test-eval"},
		"config": map[string]any{
			"agent":         map[string]any{"type": "builtin.claude-code"},
			"mcpConfigFile": mcpConfigFile,
			"sources":       sources,
		},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal eval file: %v", err)
	}

	path := filepath.Join(dir, "eval.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write eval file: %v", err)
	}

	return path
}

// makeMCPConfig writes a minimal mcp-config.yaml to dir with the given server names.
func makeMCPConfig(t *testing.T, dir string, servers []string) string {
	t.Helper()

	mcpServers := make(map[string]any)
	for _, s := range servers {
		mcpServers[s] = map[string]any{
			"command": "echo",
			"args":    []string{s},
		}
	}

	doc := map[string]any{"mcpServers": mcpServers}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatalf("failed to marshal mcp config: %v", err)
	}

	path := filepath.Join(dir, "mcp-config.yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write mcp config: %v", err)
	}

	return path
}

// taskFileContent returns a minimal Task YAML (v1alpha2) with the given mcpServer requirements.
func taskFileContent(name string, mcpServers ...string) string {
	var reqs []map[string]any
	for _, s := range mcpServers {
		srv := s
		reqs = append(reqs, map[string]any{"mcpServer": srv})
	}

	doc := map[string]any{
		"kind":       "Task",
		"apiVersion": "mcpchecker/v1alpha2",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"requires": reqs,
			"prompt":   map[string]any{"inline": "do something"},
		},
	}

	data, _ := yaml.Marshal(doc)
	return string(data)
}

// tempCacheDirFn returns a cacheDirFn that creates subdirs inside a temp root.
func tempCacheDirFn(t *testing.T, root string) func(repo, commit string) (string, error) {
	t.Helper()
	return func(repo, commit string) (string, error) {
		// Use a simplified path for tests: root/sources/<repo-last-segment>/<commit>
		parts := strings.Split(repo, "/")
		repoName := parts[len(parts)-1]
		return filepath.Join(root, "sources", repoName, commit), nil
	}
}

// installWithTempCache is a helper that calls runInstall with a temp-dir cache.
func installWithTempCache(
	t *testing.T,
	ctx context.Context,
	evalFile string,
	onlySource, onlyExtension string,
	update bool,
	fetcher source.Fetcher,
	stdin string,
	out *bytes.Buffer,
) error {
	t.Helper()
	cacheRoot := t.TempDir()
	return runInstall(ctx, evalFile, onlySource, onlyExtension, update,
		fetcher, tempCacheDirFn(t, cacheRoot), strings.NewReader(stdin), out)
}

func TestInstall_WritesLockfile(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"my-server"})
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"my-source": {"repo": "github.com/example/tasks", "ref": "main"},
	}, mcpCfg)

	fetcher := &mockFetcher{
		resolveCommit: strings.Repeat("a", 40),
		fetchHash:     "sha256:deadbeef",
		fetchFiles:    map[string]string{},
	}

	var out bytes.Buffer
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	lock, err := eval.ReadLockFile(eval.LockFilePath(evalFile))
	if err != nil {
		t.Fatalf("failed to read lockfile: %v", err)
	}

	if lock.Sources == nil {
		t.Fatal("expected sources in lockfile, got nil")
	}
	locked, ok := lock.Sources["my-source"]
	if !ok {
		t.Fatal("expected my-source in lockfile")
	}
	if locked.Commit != strings.Repeat("a", 40) {
		t.Errorf("unexpected commit: %s", locked.Commit)
	}
	if locked.Repo != "github.com/example/tasks" {
		t.Errorf("unexpected Repo: %s", locked.Repo)
	}
}

func TestInstall_SelectiveSource(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"srv"})
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"source-a": {"repo": "github.com/example/a", "ref": "main"},
		"source-b": {"repo": "github.com/example/b", "ref": "main"},
	}, mcpCfg)

	commit := strings.Repeat("a", 40)
	fetcher := &mockFetcher{
		resolveCommit: commit,
		fetchHash:     "sha256:aabb",
		fetchFiles:    map[string]string{},
	}

	var out bytes.Buffer
	if err := installWithTempCache(t, context.Background(), evalFile, "source-a", "", false, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	lock, err := eval.ReadLockFile(eval.LockFilePath(evalFile))
	if err != nil {
		t.Fatalf("failed to read lockfile: %v", err)
	}

	if _, ok := lock.Sources["source-a"]; !ok {
		t.Error("expected source-a in lockfile")
	}
	if _, ok := lock.Sources["source-b"]; ok {
		t.Error("source-b should not be in lockfile when only source-a was installed")
	}
}

func TestInstall_SelectiveExtension(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"srv"})

	doc := map[string]any{
		"kind":       "Eval",
		"apiVersion": "mcpchecker/v1alpha1",
		"metadata":   map[string]any{"name": "test-eval"},
		"config": map[string]any{
			"agent":         map[string]any{"type": "builtin.claude-code"},
			"mcpConfigFile": mcpCfg,
			"extensions": map[string]any{
				"ext-a": map[string]any{"package": "github.com/example/ext-a@v1.0.0"},
				"ext-b": map[string]any{"package": "github.com/example/ext-b@v2.0.0"},
			},
		},
	}
	data, _ := yaml.Marshal(doc)
	evalFile := filepath.Join(dir, "eval.yaml")
	if err := os.WriteFile(evalFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := installWithTempCache(t, context.Background(), evalFile, "", "ext-a", false, &mockFetcher{}, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	lock, err := eval.ReadLockFile(eval.LockFilePath(evalFile))
	if err != nil {
		t.Fatalf("failed to read lockfile: %v", err)
	}

	if _, ok := lock.Extensions["ext-a"]; !ok {
		t.Error("expected ext-a in lockfile")
	}
	if _, ok := lock.Extensions["ext-b"]; ok {
		t.Error("ext-b should not be in lockfile when only ext-a was installed")
	}
}

func TestInstall_UpdateRefreshesCommit(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"srv"})
	// serverMapping already set so no fetch/scan needed.
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"my-source": {
			"repo":          "github.com/example/tasks",
			"ref":           "main",
			"serverMapping": map[string]any{"srv": "srv"},
		},
	}, mcpCfg)

	oldCommit := strings.Repeat("0", 40)
	newCommit := strings.Repeat("1", 40)

	// Seed lockfile with the old commit.
	lockPath := eval.LockFilePath(evalFile)
	initialLock := &eval.EvalLock{
		Sources: map[string]*eval.LockedSource{
			"my-source": {Repo: "github.com/example/tasks", Ref: "main", Commit: oldCommit},
		},
	}
	if err := eval.WriteLockFile(lockPath, initialLock); err != nil {
		t.Fatal(err)
	}

	fetcher := &mockFetcher{
		resolveCommit: newCommit,
		fetchHash:     "sha256:1111",
		fetchFiles:    map[string]string{},
	}

	var out bytes.Buffer
	// Without --update: should keep old commit.
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall (no update) failed: %v", err)
	}
	lock, _ := eval.ReadLockFile(lockPath)
	if lock.Sources["my-source"].Commit != oldCommit {
		t.Errorf("without --update, expected %s, got %s", oldCommit, lock.Sources["my-source"].Commit)
	}

	// With --update: should resolve to new commit.
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", true, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall (--update) failed: %v", err)
	}
	lock, _ = eval.ReadLockFile(lockPath)
	if lock.Sources["my-source"].Commit != newCommit {
		t.Errorf("with --update, expected %s, got %s", newCommit, lock.Sources["my-source"].Commit)
	}
}

func TestInstall_InteractiveServerMapping(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"k8s-prod", "monitoring"})
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"k8s-tasks": {"repo": "github.com/example/k8s-tasks", "ref": "main"},
	}, mcpCfg)

	commit := strings.Repeat("c", 40)
	fetcher := &mockFetcher{
		resolveCommit: commit,
		fetchHash:     "sha256:cccc",
		fetchFiles: map[string]string{
			"tasks/task-1.yaml": taskFileContent("task-1", "kubernetes"),
			"tasks/task-2.yaml": taskFileContent("task-2", "kubernetes"),
		},
	}

	var out bytes.Buffer
	// User types "k8s-prod" when prompted for "kubernetes".
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, fetcher, "k8s-prod\n", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	data, err := os.ReadFile(evalFile)
	if err != nil {
		t.Fatalf("failed to read eval file: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "kubernetes") || !strings.Contains(content, "k8s-prod") {
		t.Errorf("expected serverMapping kubernetes→k8s-prod in eval.yaml, got:\n%s", content)
	}
}

func TestInstall_SkipsPromptWhenServerNamesMatch(t *testing.T) {
	dir := t.TempDir()

	// MCP config has "kubernetes" exactly — no prompt needed.
	mcpCfg := makeMCPConfig(t, dir, []string{"kubernetes"})
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"k8s-tasks": {"repo": "github.com/example/k8s-tasks", "ref": "main"},
	}, mcpCfg)

	commit := strings.Repeat("d", 40)
	fetcher := &mockFetcher{
		resolveCommit: commit,
		fetchHash:     "sha256:dddd",
		fetchFiles: map[string]string{
			"task-1.yaml": taskFileContent("task-1", "kubernetes"),
		},
	}

	var out bytes.Buffer
	// Empty stdin — no prompt should be issued.
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	data, _ := os.ReadFile(evalFile)
	if strings.Contains(string(data), "serverMapping") {
		t.Error("expected no serverMapping written when server names match exactly")
	}
}

func TestInstall_SkipsPromptWhenMappingAlreadySet(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"k8s-prod"})
	evalFile := makeEvalFile(t, dir, map[string]map[string]any{
		"k8s-tasks": {
			"repo":          "github.com/example/k8s-tasks",
			"ref":           "main",
			"serverMapping": map[string]any{"kubernetes": "k8s-prod"},
		},
	}, mcpCfg)

	commit := strings.Repeat("e", 40)
	fetcher := &mockFetcher{
		resolveCommit: commit,
		fetchHash:     "sha256:eeee",
		fetchFiles:    map[string]string{},
	}

	var out bytes.Buffer
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, fetcher, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	lock, err := eval.ReadLockFile(eval.LockFilePath(evalFile))
	if err != nil {
		t.Fatal(err)
	}
	if lock.Sources["k8s-tasks"] == nil {
		t.Error("expected k8s-tasks in lockfile")
	}
}

func TestInstall_ExtensionsRecordedInLockfile(t *testing.T) {
	dir := t.TempDir()

	mcpCfg := makeMCPConfig(t, dir, []string{"srv"})
	doc := map[string]any{
		"kind":       "Eval",
		"apiVersion": "mcpchecker/v1alpha1",
		"metadata":   map[string]any{"name": "test-eval"},
		"config": map[string]any{
			"agent":         map[string]any{"type": "builtin.claude-code"},
			"mcpConfigFile": mcpCfg,
			"extensions": map[string]any{
				"my-ext": map[string]any{"package": "github.com/mcpchecker/kubernetes-extension@v0.0.2"},
			},
		},
	}
	data, _ := yaml.Marshal(doc)
	evalFile := filepath.Join(dir, "eval.yaml")
	if err := os.WriteFile(evalFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := installWithTempCache(t, context.Background(), evalFile, "", "", false, &mockFetcher{}, "", &out); err != nil {
		t.Fatalf("runInstall failed: %v", err)
	}

	lock, err := eval.ReadLockFile(eval.LockFilePath(evalFile))
	if err != nil {
		t.Fatal(err)
	}

	ext, ok := lock.Extensions["my-ext"]
	if !ok {
		t.Fatal("expected my-ext in lockfile extensions")
	}
	if ext.Package != "github.com/mcpchecker/kubernetes-extension@v0.0.2" {
		t.Errorf("unexpected extension package: %s", ext.Package)
	}
}

func TestSuggestMapping(t *testing.T) {
	cases := []struct {
		required string
		local    []string
		want     string
	}{
		{"kubernetes", []string{"k8s-prod", "monitoring"}, "k8s-prod"},
		{"monitoring", []string{"k8s-prod", "monitoring"}, "monitoring"},
		{"unknown", []string{"alpha", "beta"}, "alpha"},
		{"db", []string{}, ""},
	}

	for _, tc := range cases {
		got := suggestMapping(tc.required, tc.local)
		if got != tc.want {
			t.Errorf("suggestMapping(%q, %v) = %q, want %q", tc.required, tc.local, got, tc.want)
		}
	}
}
