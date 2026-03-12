package eval

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
)

func TestSingleAssertionResult_Succeeded(t *testing.T) {
	tt := map[string]struct {
		result   *SingleAssertionResult
		expected bool
	}{
		"nil receiver returns true": {
			result:   nil,
			expected: true,
		},
		"passed true returns true": {
			result:   &SingleAssertionResult{Passed: true},
			expected: true,
		},
		"passed false returns false": {
			result:   &SingleAssertionResult{Passed: false},
			expected: false,
		},
		"passed false with reason returns false": {
			result:   &SingleAssertionResult{Passed: false, Reason: "some error"},
			expected: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			got := tc.result.Succeeded()
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestCompositeAssertionResult_Succeeded(t *testing.T) {
	tt := map[string]struct {
		result   *CompositeAssertionResult
		expected bool
	}{
		"all nil returns true": {
			result:   &CompositeAssertionResult{},
			expected: true,
		},
		"all passed returns true": {
			result: &CompositeAssertionResult{
				ToolsUsed:        &SingleAssertionResult{Passed: true},
				RequireAny:       &SingleAssertionResult{Passed: true},
				ToolsNotUsed:     &SingleAssertionResult{Passed: true},
				MinToolCalls:     &SingleAssertionResult{Passed: true},
				MaxToolCalls:     &SingleAssertionResult{Passed: true},
				ResourcesRead:    &SingleAssertionResult{Passed: true},
				ResourcesNotRead: &SingleAssertionResult{Passed: true},
				PromptsUsed:      &SingleAssertionResult{Passed: true},
				PromptsNotUsed:   &SingleAssertionResult{Passed: true},
				CallOrder:        &SingleAssertionResult{Passed: true},
				NoDuplicateCalls: &SingleAssertionResult{Passed: true},
			},
			expected: true,
		},
		"one failure returns false - ToolsUsed": {
			result: &CompositeAssertionResult{
				ToolsUsed: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - ToolsNotUsed": {
			result: &CompositeAssertionResult{
				ToolsNotUsed: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - RequireAny": {
			result: &CompositeAssertionResult{
				RequireAny: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - MinToolCalls": {
			result: &CompositeAssertionResult{
				MinToolCalls: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - MaxToolCalls": {
			result: &CompositeAssertionResult{
				MaxToolCalls: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - ResourcesRead": {
			result: &CompositeAssertionResult{
				ResourcesRead: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - ResourcesNotRead": {
			result: &CompositeAssertionResult{
				ResourcesNotRead: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - PromptsUsed": {
			result: &CompositeAssertionResult{
				PromptsUsed: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - PromptsNotUsed": {
			result: &CompositeAssertionResult{
				PromptsNotUsed: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - CallOrder": {
			result: &CompositeAssertionResult{
				CallOrder: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"one failure returns false - NoDuplicateCalls": {
			result: &CompositeAssertionResult{
				NoDuplicateCalls: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
		"mixed nil and passed returns true": {
			result: &CompositeAssertionResult{
				ToolsUsed:    &SingleAssertionResult{Passed: true},
				MinToolCalls: &SingleAssertionResult{Passed: true},
				// others nil
			},
			expected: true,
		},
		"mixed passed and failed returns false": {
			result: &CompositeAssertionResult{
				ToolsUsed:    &SingleAssertionResult{Passed: true},
				ToolsNotUsed: &SingleAssertionResult{Passed: false},
			},
			expected: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			got := tc.result.Succeeded()
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestCompositeAssertionResult_Counts(t *testing.T) {
	tt := map[string]struct {
		result         *CompositeAssertionResult
		expectedTotal  int
		expectedPassed int
		expectedFailed int
	}{
		"all nil": {
			result:         &CompositeAssertionResult{},
			expectedTotal:  0,
			expectedPassed: 0,
			expectedFailed: 0,
		},
		"one passed": {
			result: &CompositeAssertionResult{
				ToolsUsed: &SingleAssertionResult{Passed: true},
			},
			expectedTotal:  1,
			expectedPassed: 1,
			expectedFailed: 0,
		},
		"one failed": {
			result: &CompositeAssertionResult{
				ToolsUsed: &SingleAssertionResult{Passed: false},
			},
			expectedTotal:  1,
			expectedPassed: 0,
			expectedFailed: 1,
		},
		"two passed one failed": {
			result: &CompositeAssertionResult{
				ToolsUsed:    &SingleAssertionResult{Passed: true},
				ToolsNotUsed: &SingleAssertionResult{Passed: true},
				MinToolCalls: &SingleAssertionResult{Passed: false},
			},
			expectedTotal:  3,
			expectedPassed: 2,
			expectedFailed: 1,
		},
		"all eleven set and passed": {
			result: &CompositeAssertionResult{
				ToolsUsed:        &SingleAssertionResult{Passed: true},
				RequireAny:       &SingleAssertionResult{Passed: true},
				ToolsNotUsed:     &SingleAssertionResult{Passed: true},
				MinToolCalls:     &SingleAssertionResult{Passed: true},
				MaxToolCalls:     &SingleAssertionResult{Passed: true},
				ResourcesRead:    &SingleAssertionResult{Passed: true},
				ResourcesNotRead: &SingleAssertionResult{Passed: true},
				PromptsUsed:      &SingleAssertionResult{Passed: true},
				PromptsNotUsed:   &SingleAssertionResult{Passed: true},
				CallOrder:        &SingleAssertionResult{Passed: true},
				NoDuplicateCalls: &SingleAssertionResult{Passed: true},
			},
			expectedTotal:  11,
			expectedPassed: 11,
			expectedFailed: 0,
		},
		"all eleven set mixed results": {
			result: &CompositeAssertionResult{
				ToolsUsed:        &SingleAssertionResult{Passed: true},
				RequireAny:       &SingleAssertionResult{Passed: false},
				ToolsNotUsed:     &SingleAssertionResult{Passed: true},
				MinToolCalls:     &SingleAssertionResult{Passed: false},
				MaxToolCalls:     &SingleAssertionResult{Passed: true},
				ResourcesRead:    &SingleAssertionResult{Passed: false},
				ResourcesNotRead: &SingleAssertionResult{Passed: true},
				PromptsUsed:      &SingleAssertionResult{Passed: false},
				PromptsNotUsed:   &SingleAssertionResult{Passed: true},
				CallOrder:        &SingleAssertionResult{Passed: false},
				NoDuplicateCalls: &SingleAssertionResult{Passed: true},
			},
			expectedTotal:  11,
			expectedPassed: 6,
			expectedFailed: 5,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			assert.Equal(t, tc.expectedTotal, tc.result.TotalAssertions())
			assert.Equal(t, tc.expectedPassed, tc.result.PassedAssertions())
			assert.Equal(t, tc.expectedFailed, tc.result.FailedAssertions())
		})
	}
}

func TestToolsUsedEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions  []ToolAssertion
		history     *mcpproxy.CallHistory
		expectPass  bool
		checkReason bool
		reason      string
	}{
		"empty history fails": {
			assertions:  []ToolAssertion{{Server: "server1", Tool: "tool1"}},
			history:     &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{}},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server1, tool=tool1, pattern=",
		},
		"exact tool match passes": {
			assertions: []ToolAssertion{{Server: "server1", Tool: "tool1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool1"},
				},
			},
			expectPass: true,
		},
		"pattern match passes": {
			assertions: []ToolAssertion{{Server: "server1", ToolPattern: "tool.*"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool123"},
				},
			},
			expectPass: true,
		},
		"server only matches any tool": {
			assertions: []ToolAssertion{{Server: "server1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "anything"},
				},
			},
			expectPass: true,
		},
		"wrong server fails": {
			assertions: []ToolAssertion{{Server: "server1", Tool: "tool1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server2"}, ToolName: "tool1"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server1, tool=tool1, pattern=",
		},
		"wrong tool fails": {
			assertions: []ToolAssertion{{Server: "server1", Tool: "tool1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool2"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server1, tool=tool1, pattern=",
		},
		"multiple assertions all found": {
			assertions: []ToolAssertion{
				{Server: "server1", Tool: "tool1"},
				{Server: "server2", Tool: "tool2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool1"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "server2"}, ToolName: "tool2"},
				},
			},
			expectPass: true,
		},
		"multiple assertions first missing fails": {
			assertions: []ToolAssertion{
				{Server: "server1", Tool: "missing"},
				{Server: "server2", Tool: "tool2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server2"}, ToolName: "tool2"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server1, tool=missing, pattern=",
		},
		"multiple assertions last missing fails": {
			assertions: []ToolAssertion{
				{Server: "server1", Tool: "tool1"},
				{Server: "server2", Tool: "missing"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool1"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server2, tool=missing, pattern=",
		},
		"invalid regex does not match": {
			assertions: []ToolAssertion{{Server: "server1", ToolPattern: "[invalid"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "tool1"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "Required tool not called: server=server1, tool=, pattern=[invalid",
		},
		"pattern no match fails": {
			assertions: []ToolAssertion{{Server: "server1", ToolPattern: "^get_.*"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "server1"}, ToolName: "set_value"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewToolsUsedEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			if tc.checkReason {
				assert.Equal(t, tc.reason, result.Reason)
			}
			assert.Equal(t, assertionTypeToolsUsed, eval.Type())
		})
	}
}

func TestRequireAnyEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions  []ToolAssertion
		history     *mcpproxy.CallHistory
		expectPass  bool
		checkReason bool
		reason      string
	}{
		"empty history fails": {
			assertions:  []ToolAssertion{{Server: "s1", Tool: "t1"}},
			history:     &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{}},
			expectPass:  false,
			checkReason: true,
			reason:      "None of the required tools were called",
		},
		"first assertion matches": {
			assertions: []ToolAssertion{
				{Server: "s1", Tool: "t1"},
				{Server: "s2", Tool: "t2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "t1"},
				},
			},
			expectPass: true,
		},
		"second assertion matches": {
			assertions: []ToolAssertion{
				{Server: "s1", Tool: "t1"},
				{Server: "s2", Tool: "t2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, ToolName: "t2"},
				},
			},
			expectPass: true,
		},
		"no match fails": {
			assertions: []ToolAssertion{
				{Server: "s1", Tool: "t1"},
				{Server: "s2", Tool: "t2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s3"}, ToolName: "t3"},
				},
			},
			expectPass:  false,
			checkReason: true,
			reason:      "None of the required tools were called",
		},
		"pattern match passes": {
			assertions: []ToolAssertion{
				{Server: "s1", ToolPattern: "get_.*"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "get_user"},
				},
			},
			expectPass: true,
		},
		"server only matches any tool": {
			assertions: []ToolAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "anything"},
				},
			},
			expectPass: true,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewRequireAnyEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			if tc.checkReason {
				assert.Equal(t, tc.reason, result.Reason)
			}
			assert.Equal(t, assertionTypeRequireAny, eval.Type())
		})
	}
}

func TestToolsNotUsedEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions []ToolAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history passes": {
			assertions: []ToolAssertion{{Server: "s1", Tool: "forbidden"}},
			history:    &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{}},
			expectPass: true,
		},
		"forbidden tool used fails": {
			assertions: []ToolAssertion{{Server: "s1", Tool: "forbidden"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "forbidden"},
				},
			},
			expectPass: false,
		},
		"non-forbidden tool passes": {
			assertions: []ToolAssertion{{Server: "s1", Tool: "forbidden"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "allowed"},
				},
			},
			expectPass: true,
		},
		"pattern match forbidden fails": {
			assertions: []ToolAssertion{{Server: "s1", ToolPattern: "danger.*"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "dangerous"},
				},
			},
			expectPass: false,
		},
		"pattern no match passes": {
			assertions: []ToolAssertion{{Server: "s1", ToolPattern: "danger.*"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "safe"},
				},
			},
			expectPass: true,
		},
		"wrong server passes": {
			assertions: []ToolAssertion{{Server: "s1", Tool: "forbidden"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, ToolName: "forbidden"},
				},
			},
			expectPass: true,
		},
		"server only forbids any tool from server": {
			assertions: []ToolAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "anything"},
				},
			},
			expectPass: false,
		},
		"multiple assertions first matches fails": {
			assertions: []ToolAssertion{
				{Server: "s1", Tool: "forbidden1"},
				{Server: "s2", Tool: "forbidden2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "forbidden1"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewToolsNotUsedEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeToolsNotUsed, eval.Type())
		})
	}
}

func TestMinToolCallsEvaluator(t *testing.T) {
	tt := map[string]struct {
		min        int
		callCount  int
		expectPass bool
	}{
		"zero calls with min zero passes": {
			min:        0,
			callCount:  0,
			expectPass: true,
		},
		"zero calls with min one fails": {
			min:        1,
			callCount:  0,
			expectPass: false,
		},
		"exactly min calls passes": {
			min:        3,
			callCount:  3,
			expectPass: true,
		},
		"above min calls passes": {
			min:        3,
			callCount:  5,
			expectPass: true,
		},
		"below min calls fails": {
			min:        3,
			callCount:  2,
			expectPass: false,
		},
		"one call with min one passes": {
			min:        1,
			callCount:  1,
			expectPass: true,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			// Build history with the specified number of calls
			calls := make([]*mcpproxy.ToolCall, tc.callCount)
			for i := 0; i < tc.callCount; i++ {
				calls[i] = &mcpproxy.ToolCall{
					CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
					ToolName:   "tool",
				}
			}
			history := &mcpproxy.CallHistory{ToolCalls: calls}

			eval := NewMinToolCallsEvaluator(tc.min)
			result := eval.Evaluate(history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeMinToolCalls, eval.Type())
		})
	}
}

func TestMaxToolCallsEvaluator(t *testing.T) {
	tt := map[string]struct {
		max        int
		callCount  int
		expectPass bool
	}{
		"zero calls with any max passes": {
			max:        5,
			callCount:  0,
			expectPass: true,
		},
		"exactly max calls passes": {
			max:        3,
			callCount:  3,
			expectPass: true,
		},
		"below max calls passes": {
			max:        3,
			callCount:  2,
			expectPass: true,
		},
		"above max calls fails": {
			max:        3,
			callCount:  4,
			expectPass: false,
		},
		"zero max with zero calls passes": {
			max:        0,
			callCount:  0,
			expectPass: true,
		},
		"zero max with one call fails": {
			max:        0,
			callCount:  1,
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			// Build history with the specified number of calls
			calls := make([]*mcpproxy.ToolCall, tc.callCount)
			for i := 0; i < tc.callCount; i++ {
				calls[i] = &mcpproxy.ToolCall{
					CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
					ToolName:   "tool",
				}
			}
			history := &mcpproxy.CallHistory{ToolCalls: calls}

			eval := NewMaxToolCallsEvaluator(tc.max)
			result := eval.Evaluate(history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeMaxToolCalls, eval.Type())
		})
	}
}

func TestResourcesReadEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions []ResourceAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history fails": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "file://test"}},
			history:    &mcpproxy.CallHistory{ResourceReads: []*mcpproxy.ResourceRead{}},
			expectPass: false,
		},
		"exact uri match passes": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "file://test"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://test"},
				},
			},
			expectPass: true,
		},
		"pattern match passes": {
			assertions: []ResourceAssertion{{Server: "s1", URIPattern: "file://.*\\.txt"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://doc.txt"},
				},
			},
			expectPass: true,
		},
		"server only matches any resource": {
			assertions: []ResourceAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "anything"},
				},
			},
			expectPass: true,
		},
		"wrong server fails": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "file://test"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, URI: "file://test"},
				},
			},
			expectPass: false,
		},
		"wrong uri fails": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "file://test"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://other"},
				},
			},
			expectPass: false,
		},
		"multiple assertions all found": {
			assertions: []ResourceAssertion{
				{Server: "s1", URI: "file://a"},
				{Server: "s2", URI: "file://b"},
			},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://a"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, URI: "file://b"},
				},
			},
			expectPass: true,
		},
		"pattern no match fails": {
			assertions: []ResourceAssertion{{Server: "s1", URIPattern: "^file://.*\\.json$"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://doc.txt"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewResourcesReadEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeResourcesRead, eval.Type())
		})
	}
}

func TestResourcesNotReadEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions []ResourceAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history passes": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "secret://data"}},
			history:    &mcpproxy.CallHistory{ResourceReads: []*mcpproxy.ResourceRead{}},
			expectPass: true,
		},
		"forbidden resource read fails": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "secret://data"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "secret://data"},
				},
			},
			expectPass: false,
		},
		"non-forbidden resource passes": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "secret://data"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "public://data"},
				},
			},
			expectPass: true,
		},
		"pattern match forbidden fails": {
			assertions: []ResourceAssertion{{Server: "s1", URIPattern: "secret://.*"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "secret://password"},
				},
			},
			expectPass: false,
		},
		"wrong server passes": {
			assertions: []ResourceAssertion{{Server: "s1", URI: "secret://data"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, URI: "secret://data"},
				},
			},
			expectPass: true,
		},
		"server only forbids any resource from server": {
			assertions: []ResourceAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "anything"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewResourcesNotReadEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeResourcesNotRead, eval.Type())
		})
	}
}

func TestPromptsUsedEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions []PromptAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history fails": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "greeting"}},
			history:    &mcpproxy.CallHistory{PromptGets: []*mcpproxy.PromptGet{}},
			expectPass: false,
		},
		"exact prompt match passes": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "greeting"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
				},
			},
			expectPass: true,
		},
		"pattern match passes": {
			assertions: []PromptAssertion{{Server: "s1", PromptPattern: "greet.*"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
				},
			},
			expectPass: true,
		},
		"server only matches any prompt": {
			assertions: []PromptAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "anything"},
				},
			},
			expectPass: true,
		},
		"wrong server fails": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "greeting"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, Name: "greeting"},
				},
			},
			expectPass: false,
		},
		"wrong prompt fails": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "greeting"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "farewell"},
				},
			},
			expectPass: false,
		},
		"multiple assertions all found": {
			assertions: []PromptAssertion{
				{Server: "s1", Prompt: "greeting"},
				{Server: "s2", Prompt: "farewell"},
			},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, Name: "farewell"},
				},
			},
			expectPass: true,
		},
		"pattern no match fails": {
			assertions: []PromptAssertion{{Server: "s1", PromptPattern: "^hello.*"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "goodbye"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewPromptsUsedEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypePromptsUsed, eval.Type())
		})
	}
}

func TestPromptsNotUsedEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions []PromptAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history passes": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "dangerous"}},
			history:    &mcpproxy.CallHistory{PromptGets: []*mcpproxy.PromptGet{}},
			expectPass: true,
		},
		"forbidden prompt used fails": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "dangerous"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "dangerous"},
				},
			},
			expectPass: false,
		},
		"non-forbidden prompt passes": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "dangerous"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "safe"},
				},
			},
			expectPass: true,
		},
		"pattern match forbidden fails": {
			assertions: []PromptAssertion{{Server: "s1", PromptPattern: "danger.*"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "dangerous"},
				},
			},
			expectPass: false,
		},
		"wrong server passes": {
			assertions: []PromptAssertion{{Server: "s1", Prompt: "dangerous"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s2"}, Name: "dangerous"},
				},
			},
			expectPass: true,
		},
		"server only forbids any prompt from server": {
			assertions: []PromptAssertion{{Server: "s1"}},
			history: &mcpproxy.CallHistory{
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "anything"},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewPromptsNotUsedEvaluator(tc.assertions)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypePromptsNotUsed, eval.Type())
		})
	}
}

func TestCallOrderEvaluator(t *testing.T) {
	baseTime := time.Now()

	tt := map[string]struct {
		callOrder  []CallOrderAssertion
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history fails": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
			},
			history:    &mcpproxy.CallHistory{},
			expectPass: false,
		},
		"exact order passes": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "tool", Server: "s1", Name: "t2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t1"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "t2"},
				},
			},
			expectPass: true,
		},
		"wrong order fails": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "tool", Server: "s1", Name: "t2"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t2"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "t1"},
				},
			},
			expectPass: false,
		},
		"extra calls between expected passes": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "tool", Server: "s1", Name: "t3"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t1"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "t2"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(2 * time.Second)}, ToolName: "t3"},
				},
			},
			expectPass: true,
		},
		"mixed types in order": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "resource", Server: "s1", Name: "file://a"},
				{Type: "prompt", Server: "s1", Name: "p1"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t1"},
				},
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, URI: "file://a"},
				},
				PromptGets: []*mcpproxy.PromptGet{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(2 * time.Second)}, Name: "p1"},
				},
			},
			expectPass: true,
		},
		"mixed types wrong order fails": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "resource", Server: "s1", Name: "file://a"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "t1"},
				},
				ResourceReads: []*mcpproxy.ResourceRead{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, URI: "file://a"},
				},
			},
			expectPass: false,
		},
		"timestamps sorted correctly": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "first"},
				{Type: "tool", Server: "s1", Name: "second"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					// Added in wrong order but timestamps are correct
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "second"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "first"},
				},
			},
			expectPass: true,
		},
		"single assertion found passes": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t1"},
				},
			},
			expectPass: true,
		},
		"partial match fails": {
			callOrder: []CallOrderAssertion{
				{Type: "tool", Server: "s1", Name: "t1"},
				{Type: "tool", Server: "s1", Name: "t2"},
				{Type: "tool", Server: "s1", Name: "t3"},
			},
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime}, ToolName: "t1"},
					{CallRecord: mcpproxy.CallRecord{ServerName: "s1", Timestamp: baseTime.Add(time.Second)}, ToolName: "t2"},
					// t3 is missing
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewCallOrderEvaluator(tc.callOrder)
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeCallOrder, eval.Type())
		})
	}
}

func TestNoDuplicateCallsEvaluator(t *testing.T) {
	tt := map[string]struct {
		history    *mcpproxy.CallHistory
		expectPass bool
	}{
		"empty history passes": {
			history:    &mcpproxy.CallHistory{ToolCalls: []*mcpproxy.ToolCall{}},
			expectPass: true,
		},
		"single call passes": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
				},
			},
			expectPass: true,
		},
		"unique calls pass": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t2",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
				},
			},
			expectPass: true,
		},
		"duplicate calls fail": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
				},
			},
			expectPass: false,
		},
		"same tool different args passes": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":2}`)},
						},
					},
				},
			},
			expectPass: true,
		},
		"same tool different server passes": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s2"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: json.RawMessage(`{"a":1}`)},
						},
					},
				},
			},
			expectPass: true,
		},
		"nil arguments handled": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: nil},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t2",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: nil},
						},
					},
				},
			},
			expectPass: true,
		},
		"duplicate with nil arguments fails": {
			history: &mcpproxy.CallHistory{
				ToolCalls: []*mcpproxy.ToolCall{
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: nil},
						},
					},
					{
						CallRecord: mcpproxy.CallRecord{ServerName: "s1"},
						ToolName:   "t1",
						Request: &mcp.CallToolRequest{
							Params: &mcp.CallToolParamsRaw{Arguments: nil},
						},
					},
				},
			},
			expectPass: false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewNoDuplicateCallsEvaluator()
			result := eval.Evaluate(tc.history)

			assert.Equal(t, tc.expectPass, result.Passed)
			assert.Equal(t, assertionTypeNoDuplicateCalls, eval.Type())
		})
	}
}

func TestMatchesToolAssertion(t *testing.T) {
	tt := map[string]struct {
		call      *mcpproxy.ToolCall
		assertion ToolAssertion
		expected  bool
	}{
		"nil call returns false": {
			call:      nil,
			assertion: ToolAssertion{Server: "s1"},
			expected:  false,
		},
		"server mismatch returns false": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "t1"},
			assertion: ToolAssertion{Server: "s2", Tool: "t1"},
			expected:  false,
		},
		"no tool or pattern matches any": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "anything"},
			assertion: ToolAssertion{Server: "s1"},
			expected:  true,
		},
		"exact tool matches": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "t1"},
			assertion: ToolAssertion{Server: "s1", Tool: "t1"},
			expected:  true,
		},
		"exact tool no match": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "t1"},
			assertion: ToolAssertion{Server: "s1", Tool: "t2"},
			expected:  false,
		},
		"pattern matches": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "get_user"},
			assertion: ToolAssertion{Server: "s1", ToolPattern: "get_.*"},
			expected:  true,
		},
		"pattern no match": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "set_user"},
			assertion: ToolAssertion{Server: "s1", ToolPattern: "get_.*"},
			expected:  false,
		},
		"invalid pattern returns false": {
			call:      &mcpproxy.ToolCall{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, ToolName: "t1"},
			assertion: ToolAssertion{Server: "s1", ToolPattern: "[invalid"},
			expected:  false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			got := matchesToolAssertion(tc.call, tc.assertion)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestMatchesResourceAssertion(t *testing.T) {
	tt := map[string]struct {
		call      *mcpproxy.ResourceRead
		assertion ResourceAssertion
		expected  bool
	}{
		"nil call returns false": {
			call:      nil,
			assertion: ResourceAssertion{Server: "s1"},
			expected:  false,
		},
		"server mismatch returns false": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://a"},
			assertion: ResourceAssertion{Server: "s2", URI: "file://a"},
			expected:  false,
		},
		"no uri or pattern matches any": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "anything"},
			assertion: ResourceAssertion{Server: "s1"},
			expected:  true,
		},
		"exact uri matches": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://a"},
			assertion: ResourceAssertion{Server: "s1", URI: "file://a"},
			expected:  true,
		},
		"exact uri no match": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://a"},
			assertion: ResourceAssertion{Server: "s1", URI: "file://b"},
			expected:  false,
		},
		"pattern matches": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://doc.txt"},
			assertion: ResourceAssertion{Server: "s1", URIPattern: ".*\\.txt$"},
			expected:  true,
		},
		"pattern no match": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://doc.json"},
			assertion: ResourceAssertion{Server: "s1", URIPattern: ".*\\.txt$"},
			expected:  false,
		},
		"invalid pattern returns false": {
			call:      &mcpproxy.ResourceRead{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, URI: "file://a"},
			assertion: ResourceAssertion{Server: "s1", URIPattern: "[invalid"},
			expected:  false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			got := matchesResourceAssertion(tc.call, tc.assertion)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func TestMatchesPromptAssertion(t *testing.T) {
	tt := map[string]struct {
		call      *mcpproxy.PromptGet
		assertion PromptAssertion
		expected  bool
	}{
		"nil call returns false": {
			call:      nil,
			assertion: PromptAssertion{Server: "s1"},
			expected:  false,
		},
		"server mismatch returns false": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
			assertion: PromptAssertion{Server: "s2", Prompt: "greeting"},
			expected:  false,
		},
		"no prompt or pattern matches any": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "anything"},
			assertion: PromptAssertion{Server: "s1"},
			expected:  true,
		},
		"exact prompt matches": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
			assertion: PromptAssertion{Server: "s1", Prompt: "greeting"},
			expected:  true,
		},
		"exact prompt no match": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
			assertion: PromptAssertion{Server: "s1", Prompt: "farewell"},
			expected:  false,
		},
		"pattern matches": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting_formal"},
			assertion: PromptAssertion{Server: "s1", PromptPattern: "greeting_.*"},
			expected:  true,
		},
		"pattern no match": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "farewell"},
			assertion: PromptAssertion{Server: "s1", PromptPattern: "greeting_.*"},
			expected:  false,
		},
		"invalid pattern returns false": {
			call:      &mcpproxy.PromptGet{CallRecord: mcpproxy.CallRecord{ServerName: "s1"}, Name: "greeting"},
			assertion: PromptAssertion{Server: "s1", PromptPattern: "[invalid"},
			expected:  false,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			got := matchesPromptAssertion(tc.call, tc.assertion)
			assert.Equal(t, tc.expected, got)
		})
	}
}

func intPtr(i int) *int {
	return &i
}

func TestCompositeAssertionResult_Merge(t *testing.T) {
	passed := &SingleAssertionResult{Passed: true}
	failed := &SingleAssertionResult{Passed: false, Reason: "failed"}

	t.Run("nil receiver returns other", func(t *testing.T) {
		var c *CompositeAssertionResult
		other := &CompositeAssertionResult{ToolsUsed: passed}
		result := c.Merge(other)
		assert.Equal(t, other, result)
	})

	t.Run("nil other returns receiver", func(t *testing.T) {
		c := &CompositeAssertionResult{ToolsUsed: passed}
		result := c.Merge(nil)
		assert.Equal(t, c, result)
	})

	t.Run("merges all fields", func(t *testing.T) {
		a := &CompositeAssertionResult{
			ToolsUsed:    passed,
			RequireAny:   passed,
			ToolsNotUsed: passed,
		}
		b := &CompositeAssertionResult{
			MinToolCalls:     passed,
			MaxToolCalls:     passed,
			ResourcesRead:    passed,
			ResourcesNotRead: passed,
			PromptsUsed:      passed,
			PromptsNotUsed:   passed,
			CallOrder:        passed,
			NoDuplicateCalls: passed,
		}
		result := a.Merge(b)

		assert.Equal(t, passed, result.ToolsUsed)
		assert.Equal(t, passed, result.RequireAny)
		assert.Equal(t, passed, result.ToolsNotUsed)
		assert.Equal(t, passed, result.MinToolCalls)
		assert.Equal(t, passed, result.MaxToolCalls)
		assert.Equal(t, passed, result.ResourcesRead)
		assert.Equal(t, passed, result.ResourcesNotRead)
		assert.Equal(t, passed, result.PromptsUsed)
		assert.Equal(t, passed, result.PromptsNotUsed)
		assert.Equal(t, passed, result.CallOrder)
		assert.Equal(t, passed, result.NoDuplicateCalls)
	})

	t.Run("failure takes precedence over pass", func(t *testing.T) {
		a := &CompositeAssertionResult{ToolsUsed: passed}
		b := &CompositeAssertionResult{ToolsUsed: failed}
		result := a.Merge(b)
		assert.Equal(t, failed, result.ToolsUsed)

		// Also test reverse order
		result2 := b.Merge(a)
		assert.Equal(t, failed, result2.ToolsUsed)
	})

	t.Run("both failures combines into details", func(t *testing.T) {
		failedA := &SingleAssertionResult{Passed: false, Reason: "reason A"}
		failedB := &SingleAssertionResult{Passed: false, Reason: "reason B"}
		a := &CompositeAssertionResult{ToolsUsed: failedA}
		b := &CompositeAssertionResult{ToolsUsed: failedB}

		result := a.Merge(b)

		assert.False(t, result.ToolsUsed.Passed)
		assert.Equal(t, "multiple assertion failures", result.ToolsUsed.Reason)
		assert.Equal(t, []string{"reason A", "reason B"}, result.ToolsUsed.Details)
	})

	t.Run("both failures with existing details combines all", func(t *testing.T) {
		failedA := &SingleAssertionResult{
			Passed:  false,
			Reason:  "reason A",
			Details: []string{"detail A1", "detail A2"},
		}
		failedB := &SingleAssertionResult{
			Passed:  false,
			Reason:  "reason B",
			Details: []string{"detail B1"},
		}
		a := &CompositeAssertionResult{ToolsUsed: failedA}
		b := &CompositeAssertionResult{ToolsUsed: failedB}

		result := a.Merge(b)

		assert.False(t, result.ToolsUsed.Passed)
		assert.Equal(t, "multiple assertion failures", result.ToolsUsed.Reason)
		assert.Equal(t, []string{"reason A", "detail A1", "detail A2", "reason B", "detail B1"}, result.ToolsUsed.Details)
	})

	t.Run("both failures with empty reason skips empty", func(t *testing.T) {
		failedA := &SingleAssertionResult{Passed: false, Reason: "reason A"}
		failedB := &SingleAssertionResult{Passed: false, Reason: ""}
		a := &CompositeAssertionResult{ToolsUsed: failedA}
		b := &CompositeAssertionResult{ToolsUsed: failedB}

		result := a.Merge(b)

		assert.Equal(t, []string{"reason A"}, result.ToolsUsed.Details)
	})

	t.Run("all struct fields are handled by Merge", func(t *testing.T) {
		// This test uses reflection to ensure Merge handles all fields.
		// If a new field is added to CompositeAssertionResult, this test will fail
		// unless Merge is updated to handle it.
		typ := reflect.TypeOf(CompositeAssertionResult{})

		// Create two structs: 'a' with all fields set, 'b' empty
		a := &CompositeAssertionResult{}
		b := &CompositeAssertionResult{}
		aVal := reflect.ValueOf(a).Elem()

		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Type != reflect.TypeOf((*SingleAssertionResult)(nil)) {
				t.Fatalf("unexpected field type for %s: %v (Merge may need updating)", field.Name, field.Type)
			}
			// Set field in 'a' to a passed result
			aVal.Field(i).Set(reflect.ValueOf(&SingleAssertionResult{Passed: true, Reason: field.Name}))
		}

		result := a.Merge(b)
		resultVal := reflect.ValueOf(result).Elem()

		// Verify all fields were merged (should have 'a' values since 'b' is all nil)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fieldVal := resultVal.Field(i)
			if fieldVal.IsNil() {
				t.Errorf("field %s was not merged (is nil in result)", field.Name)
			}
		}
	})
}

func TestNewCompositeAssertionEvaluator(t *testing.T) {
	tt := map[string]struct {
		assertions            *TaskAssertions
		expectedEvaluatorCount int
	}{
		"empty assertions creates no evaluators": {
			assertions:            &TaskAssertions{},
			expectedEvaluatorCount: 0,
		},
		"single toolsUsed assertion": {
			assertions:            &TaskAssertions{ToolsUsed: []ToolAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single requireAny assertion": {
			assertions:            &TaskAssertions{RequireAny: []ToolAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single toolsNotUsed assertion": {
			assertions:            &TaskAssertions{ToolsNotUsed: []ToolAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single minToolCalls assertion": {
			assertions:            &TaskAssertions{MinToolCalls: intPtr(1)},
			expectedEvaluatorCount: 1,
		},
		"single maxToolCalls assertion": {
			assertions:            &TaskAssertions{MaxToolCalls: intPtr(10)},
			expectedEvaluatorCount: 1,
		},
		"single resourcesRead assertion": {
			assertions:            &TaskAssertions{ResourcesRead: []ResourceAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single resourcesNotRead assertion": {
			assertions:            &TaskAssertions{ResourcesNotRead: []ResourceAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single promptsUsed assertion": {
			assertions:            &TaskAssertions{PromptsUsed: []PromptAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single promptsNotUsed assertion": {
			assertions:            &TaskAssertions{PromptsNotUsed: []PromptAssertion{{Server: "s1"}}},
			expectedEvaluatorCount: 1,
		},
		"single callOrder assertion": {
			assertions:            &TaskAssertions{CallOrder: []CallOrderAssertion{{Type: "tool", Server: "s1", Name: "t1"}}},
			expectedEvaluatorCount: 1,
		},
		"single noDuplicateCalls assertion": {
			assertions:            &TaskAssertions{NoDuplicateCalls: true},
			expectedEvaluatorCount: 1,
		},
		"noDuplicateCalls false creates no evaluator": {
			assertions:            &TaskAssertions{NoDuplicateCalls: false},
			expectedEvaluatorCount: 0,
		},
		"all assertion types": {
			assertions: &TaskAssertions{
				ToolsUsed:        []ToolAssertion{{Server: "s1"}},
				RequireAny:       []ToolAssertion{{Server: "s1"}},
				ToolsNotUsed:     []ToolAssertion{{Server: "s1"}},
				MinToolCalls:     intPtr(1),
				MaxToolCalls:     intPtr(10),
				ResourcesRead:    []ResourceAssertion{{Server: "s1"}},
				ResourcesNotRead: []ResourceAssertion{{Server: "s1"}},
				PromptsUsed:      []PromptAssertion{{Server: "s1"}},
				PromptsNotUsed:   []PromptAssertion{{Server: "s1"}},
				CallOrder:        []CallOrderAssertion{{Type: "tool", Server: "s1", Name: "t1"}},
				NoDuplicateCalls: true,
			},
			expectedEvaluatorCount: 11,
		},
		"partial assertions": {
			assertions: &TaskAssertions{
				ToolsUsed:    []ToolAssertion{{Server: "s1"}},
				MinToolCalls: intPtr(1),
				MaxToolCalls: intPtr(10),
			},
			expectedEvaluatorCount: 3,
		},
	}

	for tn, tc := range tt {
		t.Run(tn, func(t *testing.T) {
			eval := NewCompositeAssertionEvaluator(tc.assertions)
			// We can't directly access the evaluators slice, so we test by evaluating
			// an empty history and counting non-nil results
			result := eval.Evaluate(&mcpproxy.CallHistory{})
			assert.Equal(t, tc.expectedEvaluatorCount, result.TotalAssertions())
		})
	}
}
