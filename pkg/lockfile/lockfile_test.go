package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testDataPath = "testdata"

func TestReadLockfile(t *testing.T) {
	basePath, err := os.Getwd()
	require.NoError(t, err)
	basePath = filepath.Join(basePath, testDataPath)

	tests := map[string]struct {
		file        string
		expectErr   bool
		errContains string
		validate    func(t *testing.T, lf *Lockfile)
	}{
		"valid lockfile with sources and extensions": {
			file: "valid.yaml",
			validate: func(t *testing.T, lf *Lockfile) {
				assert.Equal(t, 1, lf.Version)
				require.Len(t, lf.Sources, 1)
				src := lf.Sources["upstream"]
				assert.Equal(t, "github.com/org/repo", src.Repo)
				assert.Equal(t, "main", src.Ref)
				assert.Equal(t, "abc123def456789012345678901234567890abcd", src.Commit)
				assert.Equal(t, "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", src.Hash)
				assert.Equal(t, time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC), src.FetchedAt)
				require.Len(t, lf.Extensions, 1)
				ext := lf.Extensions["my-ext"]
				assert.Equal(t, "github.com/org/ext", ext.Package)
				assert.Equal(t, "v1.2.3", ext.Version)
				assert.Equal(t, "sha256:a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2", ext.Hash)
			},
		},
		"valid lockfile with sources only": {
			file: "valid-sources-only.yaml",
			validate: func(t *testing.T, lf *Lockfile) {
				require.Len(t, lf.Sources, 1)
				assert.Equal(t, "v1.0.0", lf.Sources["upstream"].Ref)
				assert.Empty(t, lf.Extensions)
			},
		},
		"valid lockfile with extensions only": {
			file: "valid-extensions-only.yaml",
			validate: func(t *testing.T, lf *Lockfile) {
				assert.Empty(t, lf.Sources)
				require.Len(t, lf.Extensions, 1)
				assert.Equal(t, "v2.0.0", lf.Extensions["my-ext"].Version)
			},
		},
		"invalid commit - too short": {
			file:        "invalid-commit-short.yaml",
			expectErr:   true,
			errContains: "invalid commit SHA",
		},
		"invalid commit - uppercase": {
			file:        "invalid-commit-uppercase.yaml",
			expectErr:   true,
			errContains: "invalid commit SHA",
		},
		"invalid commit - non-hex characters": {
			file:        "invalid-commit-non-hex.yaml",
			expectErr:   true,
			errContains: "invalid commit SHA",
		},
		"invalid commit - empty": {
			file:        "invalid-commit-empty.yaml",
			expectErr:   true,
			errContains: "invalid commit SHA",
		},
		"invalid source hash - too short": {
			file:        "invalid-hash-short.yaml",
			expectErr:   true,
			errContains: "invalid hash",
		},
		"invalid source hash - missing sha256 prefix": {
			file:        "invalid-hash-no-prefix.yaml",
			expectErr:   true,
			errContains: "invalid hash",
		},
		"invalid extension hash": {
			file:        "invalid-extension-hash.yaml",
			expectErr:   true,
			errContains: "invalid hash",
		},
		"invalid source - empty repo": {
			file:        "invalid-source-empty-repo.yaml",
			expectErr:   true,
			errContains: `source "upstream" has empty repo`,
		},
		"invalid source - empty ref": {
			file:        "invalid-source-empty-ref.yaml",
			expectErr:   true,
			errContains: `source "upstream" has empty ref`,
		},
		"invalid source - empty fetchedAt": {
			file:        "invalid-source-empty-fetchedat.yaml",
			expectErr:   true,
			errContains: "failed to parse lockfile",
		},
		"invalid source - bad fetchedAt format": {
			file:        "invalid-source-bad-fetchedat.yaml",
			expectErr:   true,
			errContains: "failed to parse lockfile",
		},
		"invalid extension - empty package": {
			file:        "invalid-extension-empty-package.yaml",
			expectErr:   true,
			errContains: `extension "my-ext" has empty package`,
		},
		"invalid extension - empty version": {
			file:        "invalid-extension-empty-version.yaml",
			expectErr:   true,
			errContains: `extension "my-ext" has empty version`,
		},
		"invalid extension - empty fetchedAt": {
			file:        "invalid-extension-empty-fetchedat.yaml",
			expectErr:   true,
			errContains: "failed to parse lockfile",
		},
		"invalid extension - bad fetchedAt format": {
			file:        "invalid-extension-bad-fetchedat.yaml",
			expectErr:   true,
			errContains: "failed to parse lockfile",
		},
		"invalid version - zero (missing)": {
			file:        "invalid-version-zero.yaml",
			expectErr:   true,
			errContains: "unsupported lockfile version: 0",
		},
		"invalid version - future": {
			file:        "invalid-version-future.yaml",
			expectErr:   true,
			errContains: "unsupported lockfile version: 2",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("%s/%s", basePath, tc.file))
			require.NoError(t, err)

			lf, err := Read(data)
			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)

			if tc.validate != nil {
				tc.validate(t, lf)
			}
		})
	}
}

func TestLockfileRoundTrip(t *testing.T) {
	basePath, err := os.Getwd()
	require.NoError(t, err)
	basePath = filepath.Join(basePath, testDataPath)

	tests := map[string]struct {
		file string
	}{
		"sources and extensions": {file: "valid.yaml"},
		"sources only":          {file: "valid-sources-only.yaml"},
		"extensions only":       {file: "valid-extensions-only.yaml"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("%s/%s", basePath, tc.file))
			require.NoError(t, err)

			original, err := Read(data)
			require.NoError(t, err)

			written, err := original.Write()
			require.NoError(t, err)

			roundTripped, err := Read(written)
			require.NoError(t, err)

			assert.Equal(t, original, roundTripped)
		})
	}
}
