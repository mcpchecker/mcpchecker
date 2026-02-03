package steps

import (
	"testing"

	"github.com/mcpchecker/mcpchecker/pkg/llmjudge"
)

func TestLLMJudgeStep_TemplateExpansion(t *testing.T) {
	tests := []struct {
		name        string
		contains    string
		exact       string
		stepOutputs map[string]map[string]string
		wantContains string
		wantExact    string
		expectErr    bool
	}{
		{
			name:     "single template substitution in contains",
			contains: "current context is {steps.kubernetes.listContexts.current}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.listContexts": {
					"current": "kind-kind",
					"count":   "3",
				},
			},
			wantContains: "current context is kind-kind",
		},
		{
			name:     "multiple template substitutions",
			contains: "current context is {steps.kubernetes.listContexts.current} with {steps.kubernetes.listContexts.count} contexts",
			stepOutputs: map[string]map[string]string{
				"kubernetes.listContexts": {
					"current": "kind-kind",
					"count":   "3",
				},
			},
			wantContains: "current context is kind-kind with 3 contexts",
		},
		{
			name:     "template not found - error",
			contains: "current context is {steps.kubernetes.listContexts.missing}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.listContexts": {
					"current": "kind-kind",
				},
			},
			expectErr: true,
		},
		{
			name:     "step type not found - error",
			contains: "value is {steps.unknown.step.field}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.listContexts": {
					"current": "kind-kind",
				},
			},
			expectErr: true,
		},
		{
			name:         "text without templates",
			contains:     "no templates here",
			stepOutputs:  map[string]map[string]string{},
			wantContains: "no templates here",
		},
		{
			name:     "multiple different step outputs",
			contains: "context {steps.kubernetes.getCurrentContext.context} has {steps.kubernetes.viewConfig.config}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.getCurrentContext": {
					"context": "production",
				},
				"kubernetes.viewConfig": {
					"config": "apiVersion: v1...",
				},
			},
			wantContains: "context production has apiVersion: v1...",
		},
		{
			name:     "templates with special characters in values",
			contains: "config: {steps.kubernetes.viewConfig.config}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.viewConfig": {
					"config": "server: https://example.com:6443",
				},
			},
			wantContains: "config: server: https://example.com:6443",
		},
		{
			name:     "exact field with template",
			exact:    "The current context is {steps.kubernetes.listContexts.current}",
			stepOutputs: map[string]map[string]string{
				"kubernetes.listContexts": {
					"current": "kind-kind",
				},
			},
			wantExact: "The current context is kind-kind",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create LLM judge step
			cfg := &llmjudge.LLMJudgeStepConfig{
				Contains: tt.contains,
				Exact:    tt.exact,
			}

			step, err := NewLLMJudgeStep(cfg)
			if err != nil {
				t.Fatalf("NewLLMJudgeStep() unexpected error = %v", err)
			}

			// Register step outputs as source
			if step.containsTemplate != nil && tt.stepOutputs != nil {
				resolver := NewStepOutputResolver(tt.stepOutputs)
				step.containsTemplate.SetSourceResolver("steps", resolver)
			}
			if step.exactTemplate != nil && tt.stepOutputs != nil {
				resolver := NewStepOutputResolver(tt.stepOutputs)
				step.exactTemplate.SetSourceResolver("steps", resolver)
			}

			// Resolve templates
			if step.containsTemplate != nil {
				result, err := step.containsTemplate.GetResult()
				if tt.expectErr {
					if err == nil {
						t.Errorf("GetResult() expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Errorf("GetResult() unexpected error = %v", err)
					return
				}
				if result != tt.wantContains {
					t.Errorf("GetResult() = %q, want %q", result, tt.wantContains)
				}
			}

			if step.exactTemplate != nil {
				result, err := step.exactTemplate.GetResult()
				if tt.expectErr {
					if err == nil {
						t.Errorf("GetResult() expected error, got nil")
					}
					return
				}
				if err != nil {
					t.Errorf("GetResult() unexpected error = %v", err)
					return
				}
				if result != tt.wantExact {
					t.Errorf("GetResult() = %q, want %q", result, tt.wantExact)
				}
			}
		})
	}
}

func TestStepOutputResolver(t *testing.T) {
	stepOutputs := map[string]map[string]string{
		"kubernetes.listContexts": {
			"current": "kind-kind",
			"count":   "3",
		},
		"kubernetes.getCurrentContext": {
			"context": "production",
		},
	}

	resolver := NewStepOutputResolver(stepOutputs)

	tests := []struct {
		name      string
		fieldName string
		want      string
		expectErr bool
	}{
		{
			name:      "resolve existing field",
			fieldName: "kubernetes.listContexts.current",
			want:      "kind-kind",
		},
		{
			name:      "resolve field with different step",
			fieldName: "kubernetes.getCurrentContext.context",
			want:      "production",
		},
		{
			name:      "missing output key",
			fieldName: "kubernetes.listContexts.missing",
			expectErr: true,
		},
		{
			name:      "missing step type",
			fieldName: "unknown.step.field",
			expectErr: true,
		},
		{
			name:      "invalid field name - no dot",
			fieldName: "nodot",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.Resolve(tt.fieldName)
			if tt.expectErr {
				if err == nil {
					t.Errorf("Resolve() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Errorf("Resolve() unexpected error = %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("Resolve() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestLLMJudgeStep_Execute_WithTemplates(t *testing.T) {
	// This is an integration test that would require a mock LLM judge
	// For now, we just test that templates are properly registered and resolved
	cfg := &llmjudge.LLMJudgeStepConfig{
		Contains: "context is {steps.kubernetes.listContexts.current}",
	}

	step, err := NewLLMJudgeStep(cfg)
	if err != nil {
		t.Fatalf("NewLLMJudgeStep() error = %v", err)
	}

	// Verify template was created
	if step.containsTemplate == nil {
		t.Error("Expected containsTemplate to be created")
	}

	// Verify we can register a resolver
	stepOutputs := map[string]map[string]string{
		"kubernetes.listContexts": {
			"current": "kind-kind",
		},
	}
	resolver := NewStepOutputResolver(stepOutputs)
	step.containsTemplate.SetSourceResolver("steps", resolver)

	// Verify template resolves correctly
	result, err := step.containsTemplate.GetResult()
	if err != nil {
		t.Errorf("GetResult() error = %v", err)
	}
	expected := "context is kind-kind"
	if result != expected {
		t.Errorf("GetResult() = %q, want %q", result, expected)
	}
}
