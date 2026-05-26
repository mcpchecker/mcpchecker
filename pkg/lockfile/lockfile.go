package lockfile

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"sigs.k8s.io/yaml"
)

const CurrentVersion = 1

var (
	commitSHARegex = regexp.MustCompile(`^[0-9a-f]{40}$`)
	hashRegex      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Lockfile struct {
	Version    int                      `json:"version"`              // version of lockfile, in case we make breaking changes in the future
	Sources    map[string]SourceLock    `json:"sources,omitempty"`    // which sources are installed
	Extensions map[string]ExtensionLock `json:"extensions,omitempty"` // which extensions are installed
}

type SourceLock struct {
	Repo      string `json:"repo"`      // repo of the source
	Ref       string `json:"ref"`       // ref in the git repo of the source
	Commit    string `json:"commit"`    // specific resolved commit of the source
	Hash      string `json:"hash"`      // hashed contents of the source, for verification of contents
	FetchedAt time.Time `json:"fetchedAt"` // time the contents were fetched
}

type ExtensionLock struct {
	Package   string    `json:"package"`   // package installed (since extensions go through package, not full repo download)
	Version   string    `json:"version"`   // version of the package
	Hash      string    `json:"hash"`      // hashed contents of the package, for verification of contents
	FetchedAt time.Time `json:"fetchedAt"` // time the contents were fetched
}

func Read(data []byte) (*Lockfile, error) {
	lf := &Lockfile{}
	if err := yaml.Unmarshal(data, lf); err != nil {
		return nil, fmt.Errorf("failed to parse lockfile: %w", err)
	}

	if err := lf.Validate(); err != nil {
		return nil, err
	}

	return lf, nil
}

func FromFile(path string) (*Lockfile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read lockfile %q: %w", path, err)
	}

	return Read(data)
}

func (lf *Lockfile) Write() ([]byte, error) {
	data, err := yaml.Marshal(lf)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal lockfile: %w", err)
	}

	return data, nil
}

func (lf *Lockfile) WriteToFile(path string) error {
	data, err := lf.Write()
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func (lf *Lockfile) Validate() error {
	if lf.Version != CurrentVersion {
		return fmt.Errorf("unsupported lockfile version: %d", lf.Version)
	}

	for name, src := range lf.Sources {
		if src.Repo == "" {
			return fmt.Errorf("source %q has empty repo", name)
		}

		if src.Ref == "" {
			return fmt.Errorf("source %q has empty ref", name)
		}

		if !commitSHARegex.MatchString(src.Commit) {
			return fmt.Errorf("source %q has invalid commit SHA %q: must be a 40-character hex string", name, src.Commit)
		}

		if !hashRegex.MatchString(src.Hash) {
			return fmt.Errorf("source %q has invalid hash %q: must be sha256:<64 hex chars>", name, src.Hash)
		}

		if src.FetchedAt.IsZero() {
			return fmt.Errorf("source %q has empty fetchedAt", name)
		}
	}

	for name, ext := range lf.Extensions {
		if ext.Package == "" {
			return fmt.Errorf("extension %q has empty package", name)
		}

		if ext.Version == "" {
			return fmt.Errorf("extension %q has empty version", name)
		}

		if !hashRegex.MatchString(ext.Hash) {
			return fmt.Errorf("extension %q has invalid hash %q: must be sha256:<64 hex chars>", name, ext.Hash)
		}

		if ext.FetchedAt.IsZero() {
			return fmt.Errorf("extension %q has empty fetchedAt", name)
		}
	}

	return nil
}
