package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	LockFileName = "mcpchecker.lock"
	LockVersion  = 1
)

// EvalLock records resolved dependency versions for an eval configuration.
type EvalLock struct {
	Version    int                          `json:"version"`
	Sources    map[string]*LockedSource     `json:"sources,omitempty"`
	Extensions map[string]*LockedExtension  `json:"extensions,omitempty"`
}

// LockedSource records the resolved commit SHA and content hash for an external task source.
type LockedSource struct {
	Repo      string    `json:"repo"`
	Ref       string    `json:"ref,omitempty"`
	Commit    string    `json:"commit"`
	Hash      string    `json:"hash,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

// LockedExtension records the resolved version and content hash for an extension.
type LockedExtension struct {
	Package   string    `json:"package"`
	Version   string    `json:"version,omitempty"`
	Hash      string    `json:"hash,omitempty"`
	FetchedAt time.Time `json:"fetchedAt,omitempty"`
}

// LockFilePath returns the lockfile path for the given eval file.
func LockFilePath(evalFilePath string) string {
	return LockFilePathFromDir(filepath.Dir(evalFilePath))
}

// LockFilePathFromDir returns the lockfile path for the given directory.
func LockFilePathFromDir(dir string) string {
	return filepath.Join(dir, LockFileName)
}

// ReadLockFile reads and parses an eval lockfile.
// Returns an empty EvalLock (not an error) if the file does not exist.
func ReadLockFile(path string) (*EvalLock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &EvalLock{}, nil
		}
		return nil, fmt.Errorf("failed to read lockfile %s: %w", path, err)
	}

	var lock EvalLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile %s: %w", path, err)
	}

	return &lock, nil
}

// WriteLockFile writes the lockfile to disk.
func WriteLockFile(path string, lock *EvalLock) error {
	if lock.Version == 0 {
		lock.Version = LockVersion
	}

	data, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("failed to marshal lockfile: %w", err)
	}

	return os.WriteFile(path, data, 0644)
}
