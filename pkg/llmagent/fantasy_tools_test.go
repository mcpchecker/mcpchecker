package llmagent

import (
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestToolsFromMcpClients(t *testing.T) {
	tests := map[string]struct {
		clients  []McpClient
		expected int
	}{
		"nil clients returns empty": {
			clients:  nil,
			expected: 0,
		},
		"empty clients returns empty": {
			clients:  []McpClient{},
			expected: 0,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tools := ToolsFromMcpClients(tc.clients, nil)
			assert.Len(t, tools, tc.expected)
		})
	}
}

func TestMcpToolInfo(t *testing.T) {
	tests := map[string]struct {
		tool     mcpsdk.Tool
		expected struct {
			name        string
			description string
			params      map[string]any
			required    []string
		}
	}{
		"basic tool with no schema": {
			tool: mcpsdk.Tool{
				Name:        "test-tool",
				Description: "A test tool",
			},
			expected: struct {
				name        string
				description string
				params      map[string]any
				required    []string
			}{
				name:        "test-tool",
				description: "A test tool",
				params:      map[string]any{},
				required:    []string{},
			},
		},
		"tool with properties and required fields": {
			tool: mcpsdk.Tool{
				Name:        "create-pod",
				Description: "Create a pod",
				InputSchema: map[string]any{
					"properties": map[string]any{
						"name":      map[string]any{"type": "string"},
						"namespace": map[string]any{"type": "string"},
					},
					"required": []any{"name"},
				},
			},
			expected: struct {
				name        string
				description string
				params      map[string]any
				required    []string
			}{
				name:        "create-pod",
				description: "Create a pod",
				params: map[string]any{
					"name":      map[string]any{"type": "string"},
					"namespace": map[string]any{"type": "string"},
				},
				required: []string{"name"},
			},
		},
		"tool with schema but no properties": {
			tool: mcpsdk.Tool{
				Name: "simple-tool",
				InputSchema: map[string]any{
					"type": "object",
				},
			},
			expected: struct {
				name        string
				description string
				params      map[string]any
				required    []string
			}{
				name:     "simple-tool",
				params:   map[string]any{},
				required: []string{},
			},
		},
		"tool with non-map schema is ignored": {
			tool: mcpsdk.Tool{
				Name:        "odd-schema",
				InputSchema: "not-a-map",
			},
			expected: struct {
				name        string
				description string
				params      map[string]any
				required    []string
			}{
				name:     "odd-schema",
				params:   map[string]any{},
				required: []string{},
			},
		},
		"required with non-string entries are skipped": {
			tool: mcpsdk.Tool{
				Name: "mixed-required",
				InputSchema: map[string]any{
					"properties": map[string]any{
						"a": map[string]any{"type": "string"},
					},
					"required": []any{"a", 42, "b"},
				},
			},
			expected: struct {
				name        string
				description string
				params      map[string]any
				required    []string
			}{
				name: "mixed-required",
				params: map[string]any{
					"a": map[string]any{"type": "string"},
				},
				required: []string{"a", "b"},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mt := &mcpTool{tool: tc.tool}
			info := mt.Info()

			assert.Equal(t, tc.expected.name, info.Name)
			assert.Equal(t, tc.expected.description, info.Description)
			assert.Equal(t, tc.expected.params, info.Parameters)
			assert.Equal(t, tc.expected.required, info.Required)
		})
	}
}
