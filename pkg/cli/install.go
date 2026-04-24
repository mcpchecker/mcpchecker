package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/mcpclient"
	"github.com/mcpchecker/mcpchecker/pkg/source"
	"github.com/spf13/cobra"
)

// NewInstallCmd creates the install command and its subcommands.
func NewInstallCmd() *cobra.Command {
	var update bool

	cmd := &cobra.Command{
		Use:   "install [eval-config-file]",
		Short: "Fetch dependencies and write lockfile",
		Long: `Fetch all sources and extensions defined in an eval config, write or update
the lockfile (mcpchecker.lock).

When installing a source for the first time, interactively maps MCP server
names from the source tasks to server names in your MCP config.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configFile := defaultEvalFile(args)
			return runInstall(cmd.Context(), configFile, "", "", update,
				&source.GitHubFetcher{}, source.SourceCacheDir, os.Stdin, os.Stdout)
		},
	}
	cmd.Flags().BoolVarP(&update, "update", "u", false, "Re-resolve all refs to current commits and update lockfile")

	sourceCmd := &cobra.Command{
		Use:   "source <name> [eval-config-file]",
		Short: "Fetch a specific source",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configFile := defaultEvalFile(args[1:])
			return runInstall(cmd.Context(), configFile, name, "", update,
				&source.GitHubFetcher{}, source.SourceCacheDir, os.Stdin, os.Stdout)
		},
	}
	sourceCmd.Flags().BoolVarP(&update, "update", "u", false, "Re-resolve ref to current commit")

	extensionCmd := &cobra.Command{
		Use:   "extension <name> [eval-config-file]",
		Short: "Fetch a specific extension",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configFile := defaultEvalFile(args[1:])
			return runInstall(cmd.Context(), configFile, "", name, update,
				&source.GitHubFetcher{}, source.SourceCacheDir, os.Stdin, os.Stdout)
		},
	}
	extensionCmd.Flags().BoolVarP(&update, "update", "u", false, "Re-resolve to current version")

	cmd.AddCommand(sourceCmd, extensionCmd)
	return cmd
}

// defaultEvalFile returns args[0] if provided, otherwise "eval.yaml".
func defaultEvalFile(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "eval.yaml"
}

// runInstall is the core install logic. fetcher and cacheDirFn are injected so
// tests can replace network/filesystem operations with mocks.
func runInstall(
	ctx context.Context,
	configFile string,
	onlySource string,
	onlyExtension string,
	update bool,
	fetcher source.Fetcher,
	cacheDirFn func(repo, commit string) (string, error),
	stdin io.Reader,
	stdout io.Writer,
) error {
	absConfigFile, err := filepath.Abs(configFile)
	if err != nil {
		return fmt.Errorf("failed to resolve config file path: %w", err)
	}

	spec, err := eval.FromFile(absConfigFile)
	if err != nil {
		return fmt.Errorf("failed to load eval config: %w", err)
	}

	lockPath := eval.LockFilePath(absConfigFile)
	lock, err := eval.ReadLockFile(lockPath)
	if err != nil {
		return fmt.Errorf("failed to read lockfile: %w", err)
	}

	// Load MCP config for server mapping prompts (optional).
	mcpConfig, _ := loadMCPConfigForInstall(spec)

	if err := installSources(ctx, absConfigFile, spec, lock, mcpConfig, onlySource, update, fetcher, cacheDirFn, stdin, stdout); err != nil {
		return err
	}

	if err := installExtensions(spec, lock, onlyExtension, stdout); err != nil {
		return err
	}

	if err := eval.WriteLockFile(lockPath, lock); err != nil {
		return fmt.Errorf("failed to write lockfile: %w", err)
	}

	fmt.Fprintf(stdout, "Wrote %s\n", lockPath)
	return nil
}

func installSources(
	ctx context.Context,
	configFile string,
	spec *eval.EvalSpec,
	lock *eval.EvalLock,
	mcpConfig *mcpclient.MCPConfig,
	onlySource string,
	update bool,
	fetcher source.Fetcher,
	cacheDirFn func(repo, commit string) (string, error),
	stdin io.Reader,
	stdout io.Writer,
) error {
	if len(spec.Config.Sources) == 0 {
		return nil
	}
	if lock.Sources == nil {
		lock.Sources = make(map[string]*eval.LockedSource)
	}

	for _, name := range sortedKeys(spec.Config.Sources) {
		if onlySource != "" && name != onlySource {
			continue
		}
		src := spec.Config.Sources[name]
		if err := installSource(ctx, configFile, name, src, lock, mcpConfig, update, fetcher, cacheDirFn, stdin, stdout); err != nil {
			return fmt.Errorf("source %q: %w", name, err)
		}
	}
	return nil
}

func installSource(
	ctx context.Context,
	configFile string,
	name string,
	src *eval.SourceSpec,
	lock *eval.EvalLock,
	mcpConfig *mcpclient.MCPConfig,
	update bool,
	fetcher source.Fetcher,
	cacheDirFn func(repo, commit string) (string, error),
	stdin io.Reader,
	stdout io.Writer,
) error {
	locked, hasLocked := lock.Sources[name]
	needsFetch := update || !hasLocked

	var commit string
	if !needsFetch {
		commit = locked.Commit
		fmt.Fprintf(stdout, "source %s: locked at %s (use --update to refresh)\n", name, shortSHA(commit))
	} else {
		fmt.Fprintf(stdout, "source %s: resolving %s ...\n", name, describeRef(src.Ref))
		var err error
		commit, err = fetcher.ResolveRef(ctx, src.Repo, src.Ref)
		if err != nil {
			return fmt.Errorf("failed to resolve ref: %w", err)
		}
		fmt.Fprintf(stdout, "source %s: resolved to %s\n", name, shortSHA(commit))
	}

	var contentHash string

	// Only fetch source content when serverMapping hasn't been configured yet.
	if len(src.ServerMapping) == 0 {
		cacheDir, err := cacheDirFn(src.Repo, commit)
		if err != nil {
			return err
		}

		existingCommit := ""
		if hasLocked {
			existingCommit = locked.Commit
		}
		if existingCommit != commit || !source.DirExists(cacheDir) {
			fmt.Fprintf(stdout, "source %s: fetching ...\n", name)
			contentHash, err = fetcher.Fetch(ctx, src.Repo, commit, cacheDir)
			if err != nil {
				return fmt.Errorf("failed to fetch source: %w", err)
			}
			source.WriteHash(cacheDir, contentHash)
		} else if hasLocked {
			contentHash = locked.Hash
		}

		scanDir := cacheDir
		if src.Path != "" {
			scanDir = filepath.Join(cacheDir, src.Path)
		}

		requiredServers, err := source.ScanTaskRequirements(scanDir)
		if err != nil {
			return fmt.Errorf("failed to scan tasks: %w", err)
		}

		if len(requiredServers) > 0 {
			mapping, err := promptServerMapping(name, requiredServers, mcpConfig, stdin, stdout)
			if err != nil {
				return err
			}
			if len(mapping) > 0 {
				if err := source.UpdateSourceServerMapping(configFile, name, mapping); err != nil {
					return fmt.Errorf("failed to update eval file with server mapping: %w", err)
				}
				fmt.Fprintf(stdout, "source %s: wrote serverMapping to %s\n", name, filepath.Base(configFile))
			}
		}
	}

	lock.Sources[name] = &eval.LockedSource{
		Repo:      src.Repo,
		Ref:       src.Ref,
		Commit:    commit,
		Hash:      contentHash,
		FetchedAt: time.Now().UTC(),
	}
	return nil
}

func installExtensions(
	spec *eval.EvalSpec,
	lock *eval.EvalLock,
	onlyExtension string,
	stdout io.Writer,
) error {
	if len(spec.Config.Extensions) == 0 {
		return nil
	}
	if lock.Extensions == nil {
		lock.Extensions = make(map[string]*eval.LockedExtension)
	}

	for _, name := range sortedKeys(spec.Config.Extensions) {
		if onlyExtension != "" && name != onlyExtension {
			continue
		}
		ext := spec.Config.Extensions[name]
		lock.Extensions[name] = &eval.LockedExtension{
			Package:   ext.Package,
			FetchedAt: time.Now().UTC(),
		}
		fmt.Fprintf(stdout, "extension %s: recorded %s\n", name, ext.Package)
	}
	return nil
}

func promptServerMapping(
	sourceName string,
	required map[string]int,
	mcpConfig *mcpclient.MCPConfig,
	stdin io.Reader,
	stdout io.Writer,
) (map[string]string, error) {
	var localServers []string
	if mcpConfig != nil {
		for name := range mcpConfig.GetEnabledServers() {
			localServers = append(localServers, name)
		}
		sort.Strings(localServers)
	}

	var needsMapping []string
	for srv := range required {
		if mcpConfig != nil {
			if _, ok := mcpConfig.GetServer(srv); ok {
				continue
			}
		}
		needsMapping = append(needsMapping, srv)
	}
	sort.Strings(needsMapping)

	if len(needsMapping) == 0 {
		return nil, nil
	}

	fmt.Fprintf(stdout, "\nDetected MCP servers required by external tasks in source %q:\n", sourceName)
	for _, srv := range sortedByCount(required) {
		fmt.Fprintf(stdout, "  - %s (%d tasks)\n", srv, required[srv])
	}

	if len(localServers) > 0 {
		fmt.Fprintf(stdout, "\nYour MCP config defines:\n")
		for _, s := range localServers {
			fmt.Fprintf(stdout, "  - %s\n", s)
		}
	}
	fmt.Fprintln(stdout)

	mapping := make(map[string]string)
	scanner := bufio.NewScanner(stdin)

	for _, srv := range needsMapping {
		defaultChoice := suggestMapping(srv, localServers)
		fmt.Fprintf(stdout, "Map %q → [%s]: ", srv, defaultChoice)

		if !scanner.Scan() {
			break
		}
		answer := strings.TrimSpace(scanner.Text())
		if answer == "" {
			answer = defaultChoice
		}
		if answer != "" {
			mapping[srv] = answer
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading input: %w", err)
	}
	return mapping, nil
}

func suggestMapping(required string, localServers []string) string {
	if len(localServers) == 0 {
		return ""
	}
	req := strings.ToLower(required)
	for _, local := range localServers {
		loc := strings.ToLower(local)
		if strings.Contains(loc, req) || strings.Contains(req, loc) {
			return local
		}
	}
	return localServers[0]
}

func loadMCPConfigForInstall(spec *eval.EvalSpec) (*mcpclient.MCPConfig, error) {
	if spec.Config.McpConfigFile == "" {
		return nil, nil
	}
	return mcpclient.ParseConfigFile(spec.Config.McpConfigFile)
}

func describeRef(ref string) string {
	if ref == "" {
		return "HEAD"
	}
	return ref
}

func shortSHA(sha string) string {
	if len(sha) >= 8 {
		return sha[:8]
	}
	return sha
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedByCount(m map[string]int) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Slice(names, func(i, j int) bool {
		if m[names[i]] != m[names[j]] {
			return m[names[i]] > m[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}
