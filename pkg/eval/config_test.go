package eval

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testDataPath = "testdata"
)

func TestReadSourceSpec(t *testing.T) {
	basePath, err := os.Getwd()
	require.NoError(t, err)
	basePath = filepath.Join(basePath, testDataPath)

	tests := map[string]struct {
		file        string
		expectErr   bool
		errContains string
		validate    func(t *testing.T, spec *EvalSpec)
	}{
		"valid config with sources": {
			file: "source-valid.yaml",
			validate: func(t *testing.T, spec *EvalSpec) {
				require.Len(t, spec.Config.Sources, 1)
				src := spec.Config.Sources["upstream"]
				assert.Equal(t, "github.com/org/repo", src.Repo)
				assert.Equal(t, "main", src.Ref)
				require.Len(t, spec.Config.TaskSets, 1)
				assert.Equal(t, "upstream", spec.Config.TaskSets[0].Source)
				assert.Equal(t, "tasks/*.yaml", spec.Config.TaskSets[0].Glob)
			},
		},
		"valid config with source and serverMapping": {
			file: "source-with-server-mapping.yaml",
			validate: func(t *testing.T, spec *EvalSpec) {
				src := spec.Config.Sources["upstream"]
				assert.Equal(t, map[string]string{"their-server": "my-server"}, src.ServerMapping)
				assert.Equal(t, "upstream", spec.Config.TaskSets[0].Source)
			},
		},
		"valid config without sources": {
			file: "source-no-sources.yaml",
			validate: func(t *testing.T, spec *EvalSpec) {
				assert.Nil(t, spec.Config.Sources)
				assert.Empty(t, spec.Config.TaskSets[0].Source)
			},
		},
		"multiple sources": {
			file: "source-multiple.yaml",
			validate: func(t *testing.T, spec *EvalSpec) {
				require.Len(t, spec.Config.Sources, 2)
				assert.Equal(t, "github.com/org/repo-a", spec.Config.Sources["upstream"].Repo)
				assert.Equal(t, "github.com/org/repo-b", spec.Config.Sources["other"].Repo)
				assert.Equal(t, "upstream", spec.Config.TaskSets[0].Source)
				assert.Equal(t, "other", spec.Config.TaskSets[1].Source)
			},
		},
		"sourced taskSet with relative path within repo": {
			file: "source-relative-path-within-repo.yaml",
			validate: func(t *testing.T, spec *EvalSpec) {
				assert.Equal(t, "upstream", spec.Config.TaskSets[0].Source)
				assert.Equal(t, "subdir/../tasks/test.yaml", spec.Config.TaskSets[0].Path)
			},
		},
		"source missing repo": {
			file:        "source-missing-repo.yaml",
			expectErr:   true,
			errContains: `source "upstream" requires a repo field`,
		},
		"source missing ref": {
			file:        "source-missing-ref.yaml",
			expectErr:   true,
			errContains: `source "upstream" requires a ref field`,
		},
		"taskSet references undefined source": {
			file:        "source-undefined-ref.yaml",
			expectErr:   true,
			errContains: `references undefined source "nonexistent"`,
		},
		"taskSet references source but no sources defined": {
			file:        "source-no-sources-defined.yaml",
			expectErr:   true,
			errContains: "no sources are defined",
		},
		"sourced taskSet with absolute path": {
			file:        "source-absolute-path.yaml",
			expectErr:   true,
			errContains: "is absolute; sourced task sets must use relative paths",
		},
		"sourced taskSet with absolute glob": {
			file:        "source-absolute-glob.yaml",
			expectErr:   true,
			errContains: "is absolute; sourced task sets must use relative paths",
		},
		"sourced taskSet with path escaping repo root": {
			file:        "source-path-escape.yaml",
			expectErr:   true,
			errContains: "escapes source repo root",
		},
		"sourced taskSet with glob escaping repo root": {
			file:        "source-glob-escape.yaml",
			expectErr:   true,
			errContains: "escapes source repo root",
		},
		"sourced taskSet with disguised path escape": {
			file:        "source-disguised-path-escape.yaml",
			expectErr:   true,
			errContains: "escapes source repo root",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("%s/%s", basePath, tc.file))
			require.NoError(t, err)

			spec, err := Read(data, basePath)
			if tc.expectErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)

			if tc.validate != nil {
				tc.validate(t, spec)
			}
		})
	}
}
