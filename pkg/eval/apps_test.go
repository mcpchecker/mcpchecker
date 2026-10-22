package eval

import (
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/mcpapps"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"
)

// A missing invocation fails, and a later success cannot hide an earlier failure.
func TestAppAssertions(t *testing.T) {
	// Parse the public YAML shape and retain Apps-only assertion sets.
	spec, err := Read([]byte(`kind: Eval
apiVersion: mcpchecker/v1alpha2
config:
  taskSets:
    - path: ./namespaces.yaml
      assertions:
        apps:
          - server: kubernetes
            tool: namespaces_list
`), t.TempDir())
	require.NoError(t, err)
	require.False(t, spec.Config.Apps.Host, "Apps assertions do not require or enable host emulation")
	require.False(t, spec.Config.TaskSets[0].Assertions.IsEmpty())
	expected := spec.Config.TaskSets[0].Assertions.Apps
	observation := mcpapps.Observation{Server: "kubernetes", Tool: "namespaces_list", CallID: "app-1", URI: "ui://namespaces", MIMEType: mcpapps.MIMEType}
	require.True(t, evaluateApps(expected, []mcpapps.Observation{observation}).Passed)
	notInvoked := evaluateApps(expected, nil)
	require.False(t, notInvoked.Passed, "missing invocations for present targets fail")
	require.Contains(t, notInvoked.Reason, "tool_not_invoked")
	failed := observation
	failed.CallID = "app-2"
	failed.Code = "missing_ui_resource"
	failed.Detail = "HTTP 403: permission denied"
	result := evaluateApps(expected, []mcpapps.Observation{failed, observation})
	require.False(t, result.Passed)
	require.Contains(t, result.Reason, "app-2")
	require.Contains(t, result.Reason, "ui://namespaces")
	require.Contains(t, result.Reason, failed.Detail)
	composite := (&CompositeAssertionResult{Apps: result}).Merge(&CompositeAssertionResult{Apps: &SingleAssertionResult{Passed: true}})
	require.False(t, composite.Succeeded())
	require.Equal(t, 2, composite.TotalAssertions())
	require.Equal(t, 1, composite.PassedAssertions())
	history := &mcpproxy.CallHistory{ResourceReads: []*mcpproxy.ResourceRead{{HostUIRead: true, URI: "ui://namespaces", CallRecord: mcpproxy.CallRecord{ServerName: "kubernetes", Success: true}}}}
	resources := NewResourcesReadEvaluator([]ResourceAssertion{{Server: "kubernetes", URI: "ui://namespaces"}}).Evaluate(history)
	require.False(t, resources.Passed, "host reads cannot satisfy agent resource assertions")

	recorder := mcpproxy.NewRecorder("kubernetes")
	recorder.RecordResourceRead(&mcp.ReadResourceRequest{Params: &mcp.ReadResourceParams{
		URI: "ui://namespaces", Meta: mcp.Meta{"mcpchecker/host-ui-read": "spoofed"},
	}}, &mcp.ReadResourceResult{}, nil, time.Time{})
	agentHistory := recorder.GetHistory()
	forbidden := NewResourcesNotReadEvaluator([]ResourceAssertion{{Server: "kubernetes", URI: "ui://namespaces"}}).Evaluate(&agentHistory)
	require.False(t, forbidden.Passed, "agent metadata cannot bypass resourcesNotRead")
}

func TestAppAssertionSets(t *testing.T) {
	first := mcpapps.Assertion{Server: "kubernetes", Tool: "namespaces_list"}
	for _, test := range []struct {
		name       string
		second     mcpapps.Assertion
		code       string
		missing    bool
		wantPassed int
	}{
		{"different tools", mcpapps.Assertion{Server: "kubernetes", Tool: "pod_get"}, "missing_ui_resource", false, 1},
		{"different servers", mcpapps.Assertion{Server: "other", Tool: "namespaces_list"}, "missing_ui_resource", false, 1},
		{"overlapping selectors", first, "", false, 2},
		{"missing second invocation", mcpapps.Assertion{Server: "kubernetes", Tool: "pod_get"}, "", true, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := &EvalResult{AppObservations: []mcpapps.Observation{{Server: first.Server, Tool: first.Tool, CallID: "app-1", URI: "ui://first", MIMEType: mcpapps.MIMEType}}}
			if !test.missing {
				result.AppObservations = append(result.AppObservations, mcpapps.Observation{Server: test.second.Server, Tool: test.second.Tool, CallID: "app-2", URI: "ui://second", MIMEType: mcpapps.MIMEType, Code: test.code})
			}
			config := taskConfig{assertions: []*TaskAssertions{{Apps: []mcpapps.Assertion{first}}, {Apps: []mcpapps.Assertion{test.second}}}}
			manager := &capabilityTestManager{servers: []mcpproxy.Server{
				&capabilityTestServer{name: "kubernetes", tools: []*mcp.Tool{{Name: "namespaces_list"}, {Name: "pod_get"}}},
				&capabilityTestServer{name: "other", tools: []*mcp.Tool{{Name: "namespaces_list"}}},
			}}
			require.NoError(t, (&evalRunner{}).evaluateTaskAssertions(t.Context(), config, manager, result))
			require.Equal(t, 2, result.AssertionResults.TotalAssertions(), "each configured assertion set counts independently")
			require.Equal(t, test.wantPassed, result.AssertionResults.PassedAssertions())
			require.Equal(t, test.wantPassed == 2, result.AllAssertionsPassed)
			if test.missing {
				require.Contains(t, result.AssertionResults.Apps.Reason, "tool_not_invoked")
			}
		})
	}
}
