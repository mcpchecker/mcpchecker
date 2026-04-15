package testcase

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"sigs.k8s.io/yaml"

	"github.com/mcpchecker/mcpchecker/functional/servers/agent"
	"github.com/mcpchecker/mcpchecker/pkg/eval"
	"github.com/mcpchecker/mcpchecker/pkg/results"
	"github.com/mcpchecker/mcpchecker/pkg/task"
	"github.com/mcpchecker/mcpchecker/pkg/util"
)

// GeneratedFiles holds paths to all generated configuration files
type GeneratedFiles struct {
	TempDir       string
	TaskFile      string
	EvalFile      string
	MCPConfigFile string
	AgentConfig   string
	OutputFile    string
}

// Generator handles generating configuration files for a test case
type Generator struct {
	t       *testing.T
	tempDir string
}

// NewGenerator creates a new generator with a temporary directory
func NewGenerator(t *testing.T) (*Generator, error) {
	tempDir, err := os.MkdirTemp("", "mcpchecker-functional-*")
	if err != nil {
		return nil, err
	}

	t.Cleanup(func() {
		os.RemoveAll(tempDir)
	})

	return &Generator{
		t:       t,
		tempDir: tempDir,
	}, nil
}

// TempDir returns the temporary directory path
func (g *Generator) TempDir() string {
	return g.tempDir
}

// GenerateTaskYAML writes a legacy task config to a YAML file (v1alpha1 format)
func (g *Generator) GenerateTaskYAML(taskConfig *TaskConfig) (string, error) {
	// Wrap in kind structure for proper deserialization
	wrapper := map[string]any{
		"apiVersion": util.APIVersionV1Alpha1,
		"kind":       task.KindTask,
		"metadata":   taskConfig.Metadata(),
		"steps":      g.buildLegacyTaskSteps(taskConfig.Steps()),
	}

	return g.writeYAML("task.yaml", wrapper)
}

// GenerateTaskYAMLV2 writes a new-format task config to a YAML file
func (g *Generator) GenerateTaskYAMLV2(taskConfig *TaskConfigV2) (string, error) {
	spec := taskConfig.Build()

	wrapper := map[string]any{
		"apiVersion": util.APIVersionV1Alpha2,
		"kind":       task.KindTask,
		"metadata":   taskConfig.Metadata(),
		"spec":       spec,
	}

	return g.writeYAML("task.yaml", wrapper)
}

func (g *Generator) buildLegacyTaskSteps(legacySteps *task.TaskStepsV1Alpha1) map[string]any {
	steps := make(map[string]any)

	if legacySteps == nil {
		return steps
	}

	if legacySteps.Prompt != nil {
		if legacySteps.Prompt.Inline != "" {
			steps["prompt"] = map[string]any{"inline": legacySteps.Prompt.Inline}
		} else if legacySteps.Prompt.File != "" {
			steps["prompt"] = map[string]any{"file": legacySteps.Prompt.File}
		}
	}

	if legacySteps.SetupScript != nil {
		if legacySteps.SetupScript.Inline != "" {
			steps["setup"] = map[string]any{"inline": legacySteps.SetupScript.Inline}
		} else if legacySteps.SetupScript.File != "" {
			steps["setup"] = map[string]any{"file": legacySteps.SetupScript.File}
		}
	}

	if legacySteps.CleanupScript != nil {
		if legacySteps.CleanupScript.Inline != "" {
			steps["cleanup"] = map[string]any{"inline": legacySteps.CleanupScript.Inline}
		} else if legacySteps.CleanupScript.File != "" {
			steps["cleanup"] = map[string]any{"file": legacySteps.CleanupScript.File}
		}
	}

	if legacySteps.VerifyScript != nil {
		verify := make(map[string]any)
		if legacySteps.VerifyScript.Step != nil {
			if legacySteps.VerifyScript.Step.Inline != "" {
				verify["inline"] = legacySteps.VerifyScript.Step.Inline
			} else if legacySteps.VerifyScript.Step.File != "" {
				verify["file"] = legacySteps.VerifyScript.Step.File
			}
		}
		if legacySteps.VerifyScript.LLMJudgeStepConfig != nil {
			if legacySteps.VerifyScript.LLMJudgeStepConfig.Contains != "" {
				verify["contains"] = legacySteps.VerifyScript.LLMJudgeStepConfig.Contains
			}
			if legacySteps.VerifyScript.LLMJudgeStepConfig.Exact != "" {
				verify["exact"] = legacySteps.VerifyScript.LLMJudgeStepConfig.Exact
			}
		}
		if len(verify) > 0 {
			steps["verify"] = verify
		}
	}

	return steps
}

// GenerateEvalYAML writes an eval spec to a YAML file
func (g *Generator) GenerateEvalYAML(evalSpec *eval.EvalSpec) (string, error) {
	// Wrap in kind structure for proper deserialization
	wrapper := map[string]any{
		"kind":     eval.KindEval,
		"metadata": evalSpec.Metadata,
		"config":   evalSpec.Config,
	}

	return g.writeYAML("eval.yaml", wrapper)
}

// GenerateMCPConfigJSON writes an MCP server configuration to a JSON file.
// This generates the config format expected by the agent (mcpServers map).
func (g *Generator) GenerateMCPConfigJSON(servers map[string]string) (string, error) {
	config := map[string]any{
		"mcpServers": make(map[string]any),
	}

	for name, url := range servers {
		config["mcpServers"].(map[string]any)[name] = map[string]any{
			"url": url,
		}
	}

	return g.writeJSON("mcp-config.json", config)
}

// GenerateAgentConfigJSON writes a mock agent configuration to a JSON file
func (g *Generator) GenerateAgentConfigJSON(agentConfig *agent.Config) (string, error) {
	return g.writeJSON("agent-config.json", agentConfig)
}

// writeYAML writes data as YAML to a file in the temp directory
func (g *Generator) writeYAML(filename string, data any) (string, error) {
	path := filepath.Join(g.tempDir, filename)

	yamlBytes, err := yaml.Marshal(data)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, yamlBytes, 0644); err != nil {
		return "", err
	}

	return path, nil
}

// writeJSON writes data as JSON to a file in the temp directory
func (g *Generator) writeJSON(filename string, data any) (string, error) {
	path := filepath.Join(g.tempDir, filename)

	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(path, jsonBytes, 0644); err != nil {
		return "", err
	}

	return path, nil
}

// ReadEvalResults reads and parses the eval output JSON file.
// Supports both the current format (object with summary + results) and
// the legacy format (bare array of results).
func ReadEvalResults(path string) ([]*eval.EvalResult, error) {
	return results.Load(path)
}

// WriteFile writes content to a file in the temp directory
func (g *Generator) WriteFile(filename, content string) (string, error) {
	path := filepath.Join(g.tempDir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", err
	}
	return path, nil
}

// Mkdir creates a subdirectory in the temp directory
func (g *Generator) Mkdir(name string) (string, error) {
	path := filepath.Join(g.tempDir, name)
	if err := os.MkdirAll(path, 0755); err != nil {
		return "", err
	}
	return path, nil
}

// writeTaskYAML writes a single task config to a YAML file.
// The caller is responsible for providing a unique basename to avoid collisions.
func (g *Generator) writeTaskYAML(basename string, taskConfig *TaskConfig) (string, error) {
	wrapper := map[string]any{
		"apiVersion": util.APIVersionV1Alpha1,
		"kind":       task.KindTask,
		"metadata":   taskConfig.Metadata(),
		"steps":      g.buildLegacyTaskSteps(taskConfig.Steps()),
	}

	return g.writeYAML(basename, wrapper)
}
