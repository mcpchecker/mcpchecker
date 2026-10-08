package mcpproxy

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestErrorToString(t *testing.T) {
	tests := map[string]struct {
		input    error
		expected string
	}{
		"nil error returns empty string": {
			input:    nil,
			expected: "",
		},
		"non-nil error returns message": {
			input:    errors.New("something failed"),
			expected: "something failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := errorToString(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestSafeServerRequestFromUnsafe(t *testing.T) {
	tests := map[string]struct {
		input         *mcp.ServerRequest[*mcp.CallToolParamsRaw]
		expectedNil   bool
		expectedExtra bool
	}{
		"nil request returns nil": {
			input:       nil,
			expectedNil: true,
		},
		"request without Extra": {
			input: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
				Extra:  nil,
			},
			expectedNil:   false,
			expectedExtra: false,
		},
		"request with Extra containing Header": {
			input: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
				Extra: &mcp.RequestExtra{
					Header: http.Header{"Authorization": []string{"Bearer token"}},
				},
			},
			expectedExtra: true,
		},
		"request with Extra containing CloseSSEStream is filtered": {
			input: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
				Extra: &mcp.RequestExtra{
					Header:         http.Header{"X-Custom": []string{"value"}},
					CloseSSEStream: func(mcp.CloseSSEStreamArgs) {}, // non-serializable
				},
			},
			expectedExtra: true,
		},
		"Extra with only CloseSSEStream set": {
			input: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
				Extra: &mcp.RequestExtra{
					CloseSSEStream: func(mcp.CloseSSEStreamArgs) {},
				},
			},
			expectedExtra: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := SafeServerRequestFromUnsafe(tc.input)

			if tc.expectedNil {
				assert.Nil(t, result)
				return
			}

			assert.NotNil(t, result)
			assert.Equal(t, tc.input.Params, result.Params)

			if tc.expectedExtra {
				assert.NotNil(t, result.Extra)
				data, err := json.Marshal(result)
				require.NoError(t, err)
				assert.NotContains(t, string(data), "Header")
			} else {
				assert.Nil(t, result.Extra)
			}
		})
	}
}

func TestRecordedRequestsOmitHeaders(t *testing.T) {
	headers := http.Header{
		"Authorization": []string{"Bearer secret-token"},
		"Cookie":        []string{"session=secret-cookie"},
		"X-Api-Key":     []string{"secret-key"},
	}
	tests := map[string]struct {
		record any
	}{
		"tool call": {
			record: &ToolCall{Request: &mcp.CallToolRequest{
				Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
				Extra:  &mcp.RequestExtra{Header: headers},
			}},
		},
		"resource read": {
			record: &ResourceRead{Request: &mcp.ReadResourceRequest{
				Params: &mcp.ReadResourceParams{URI: "file:///test"},
				Extra:  &mcp.RequestExtra{Header: headers},
			}},
		},
		"prompt get": {
			record: &PromptGet{Request: &mcp.GetPromptRequest{
				Params: &mcp.GetPromptParams{Name: "test-prompt"},
				Extra:  &mcp.RequestExtra{Header: headers},
			}},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(tc.record)
			require.NoError(t, err)

			var result map[string]any
			require.NoError(t, json.Unmarshal(data, &result))
			request := result["request"].(map[string]any)
			assert.NotEmpty(t, request["Params"])
			extra := request["Extra"].(map[string]any)
			assert.NotContains(t, extra, "Header")
			assert.NotContains(t, string(data), "secret-token")
			assert.NotContains(t, string(data), "secret-cookie")
			assert.NotContains(t, string(data), "secret-key")
			assert.Equal(t, "Bearer secret-token", headers.Get("Authorization"))
		})
	}
}

func TestToolCallMarshalJSON(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		input         *ToolCall
		checkFields   map[string]any
		presentFields []string
		absentFields  []string
	}{
		"basic fields are marshaled": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				ToolName: "my-tool",
			},
			checkFields: map[string]any{
				"serverName": "test-server",
				"success":    true,
				"name":       "my-tool",
			},
		},
		"error field omitted when empty": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
					Error:      "",
				},
				ToolName: "my-tool",
			},
			absentFields: []string{"error"},
		},
		"error field present when set": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    false,
					Error:      "connection timeout",
				},
				ToolName: "my-tool",
			},
			checkFields: map[string]any{
				"success": false,
				"error":   "connection timeout",
			},
		},
		"nil request is omitted": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				ToolName: "my-tool",
				Request:  nil,
			},
			absentFields: []string{"request"},
		},
		"nil result is omitted": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				ToolName: "my-tool",
				Result:   nil,
			},
			absentFields: []string{"result"},
		},
		"result present when set": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				ToolName: "my-tool",
				Result: &mcp.CallToolResult{
					IsError: false,
				},
			},
			presentFields: []string{"result"},
		},
		"request with CloseSSEStream is safely wrapped": {
			input: &ToolCall{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				ToolName: "my-tool",
				Request: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
					Params: &mcp.CallToolParamsRaw{Name: "test-tool"},
					Extra: &mcp.RequestExtra{
						Header:         http.Header{"X-Test": []string{"value"}},
						CloseSSEStream: func(mcp.CloseSSEStreamArgs) {},
					},
				},
			},
			presentFields: []string{"request"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(tc.input)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			for field, expected := range tc.checkFields {
				assert.Equal(t, expected, result[field], "field %s mismatch", field)
			}

			for _, field := range tc.presentFields {
				_, exists := result[field]
				assert.True(t, exists, "field %s should be present", field)
			}

			for _, field := range tc.absentFields {
				_, exists := result[field]
				assert.False(t, exists, "field %s should not be present", field)
			}
		})
	}
}

func TestResourceReadMarshalJSON(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		input         *ResourceRead
		checkFields   map[string]any
		presentFields []string
		absentFields  []string
	}{
		"basic fields are marshaled": {
			input: &ResourceRead{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				URI: "file:///tmp/test.txt",
			},
			checkFields: map[string]any{
				"serverName": "test-server",
				"success":    true,
				"uri":        "file:///tmp/test.txt",
			},
		},
		"request wrapped with SafeServerRequest": {
			input: &ResourceRead{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				URI: "file:///tmp/test.txt",
				Request: &mcp.ServerRequest[*mcp.ReadResourceParams]{
					Params: &mcp.ReadResourceParams{URI: "file:///tmp/test.txt"},
					Extra: &mcp.RequestExtra{
						Header:         http.Header{"X-Test": []string{"value"}},
						CloseSSEStream: func(mcp.CloseSSEStreamArgs) {},
					},
				},
			},
			presentFields: []string{"request"},
		},
		"nil request handling": {
			input: &ResourceRead{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				URI:     "file:///tmp/test.txt",
				Request: nil,
			},
			absentFields: []string{"request"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(tc.input)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			for field, expected := range tc.checkFields {
				assert.Equal(t, expected, result[field], "field %s mismatch", field)
			}

			for _, field := range tc.presentFields {
				_, exists := result[field]
				assert.True(t, exists, "field %s should be present", field)
			}

			for _, field := range tc.absentFields {
				_, exists := result[field]
				assert.False(t, exists, "field %s should not be present", field)
			}
		})
	}
}

func TestPromptGetMarshalJSON(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		input         *PromptGet
		checkFields   map[string]any
		presentFields []string
	}{
		"basic fields are marshaled": {
			input: &PromptGet{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				Name: "greeting-prompt",
			},
			checkFields: map[string]any{
				"serverName": "test-server",
				"success":    true,
				"name":       "greeting-prompt",
			},
		},
		"request wrapped with SafeServerRequest": {
			input: &PromptGet{
				CallRecord: CallRecord{
					ServerName: "test-server",
					Timestamp:  fixedTime,
					Success:    true,
				},
				Name: "greeting-prompt",
				Request: &mcp.ServerRequest[*mcp.GetPromptParams]{
					Params: &mcp.GetPromptParams{Name: "greeting-prompt"},
					Extra: &mcp.RequestExtra{
						Header:         http.Header{"X-Test": []string{"value"}},
						CloseSSEStream: func(mcp.CloseSSEStreamArgs) {},
					},
				},
			},
			presentFields: []string{"request"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(tc.input)
			require.NoError(t, err)

			var result map[string]any
			err = json.Unmarshal(data, &result)
			require.NoError(t, err)

			for field, expected := range tc.checkFields {
				assert.Equal(t, expected, result[field], "field %s mismatch", field)
			}

			for _, field := range tc.presentFields {
				_, exists := result[field]
				assert.True(t, exists, "field %s should be present", field)
			}
		})
	}
}

func TestNewRecorder(t *testing.T) {
	tests := map[string]struct {
		serverName string
	}{
		"creates recorder with server name": {
			serverName: "my-server",
		},
		"creates recorder with empty server name": {
			serverName: "",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := NewRecorder(tc.serverName)
			require.NotNil(t, rec)

			history := rec.GetHistory()
			assert.NotNil(t, history.ToolCalls)
			assert.NotNil(t, history.ResourceReads)
			assert.NotNil(t, history.PromptGets)
			assert.Len(t, history.ToolCalls, 0)
			assert.Len(t, history.ResourceReads, 0)
			assert.Len(t, history.PromptGets, 0)
		})
	}
}

func TestRecorderRecordToolCall(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		serverName      string
		request         *mcp.CallToolRequest
		result          *mcp.CallToolResult
		err             error
		expectedSuccess bool
		expectedError   string
		expectedTool    string
	}{
		"successful call": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "my-tool"},
			},
			result:          &mcp.CallToolResult{IsError: false},
			err:             nil,
			expectedSuccess: true,
			expectedError:   "",
			expectedTool:    "my-tool",
		},
		"failed call": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "failing-tool"},
			},
			result:          nil,
			err:             errors.New("connection timeout"),
			expectedSuccess: false,
			expectedError:   "connection timeout",
			expectedTool:    "failing-tool",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := NewRecorder(tc.serverName)
			rec.RecordToolCall(tc.request, tc.result, tc.err, fixedTime)

			history := rec.GetHistory()
			require.Len(t, history.ToolCalls, 1)

			call := history.ToolCalls[0]
			assert.Equal(t, tc.serverName, call.ServerName)
			assert.Equal(t, fixedTime, call.Timestamp)
			assert.Equal(t, tc.expectedSuccess, call.Success)
			assert.Equal(t, tc.expectedError, call.Error)
			assert.Equal(t, tc.expectedTool, call.ToolName)
			assert.Equal(t, tc.request, call.Request)
			assert.Equal(t, tc.result, call.Result)
		})
	}
}

func TestRecorderRecordToolCallAggregation(t *testing.T) {
	rec := NewRecorder("test-server")
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	// Record 3 calls
	for i := 0; i < 3; i++ {
		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
			Params: &mcp.CallToolParamsRaw{Name: "tool-" + string(rune('a'+i))},
		}
		rec.RecordToolCall(req, nil, nil, fixedTime)
	}

	history := rec.GetHistory()
	assert.Len(t, history.ToolCalls, 3)
	assert.Equal(t, "tool-a", history.ToolCalls[0].ToolName)
	assert.Equal(t, "tool-b", history.ToolCalls[1].ToolName)
	assert.Equal(t, "tool-c", history.ToolCalls[2].ToolName)
}

func TestRecorderRecordResourceRead(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		serverName      string
		request         *mcp.ReadResourceRequest
		result          *mcp.ReadResourceResult
		err             error
		expectedSuccess bool
		expectedError   string
		expectedURI     string
	}{
		"successful read": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.ReadResourceParams]{
				Params: &mcp.ReadResourceParams{URI: "file:///tmp/test.txt"},
			},
			result:          &mcp.ReadResourceResult{},
			err:             nil,
			expectedSuccess: true,
			expectedError:   "",
			expectedURI:     "file:///tmp/test.txt",
		},
		"failed read": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.ReadResourceParams]{
				Params: &mcp.ReadResourceParams{URI: "file:///nonexistent"},
			},
			result:          nil,
			err:             errors.New("file not found"),
			expectedSuccess: false,
			expectedError:   "file not found",
			expectedURI:     "file:///nonexistent",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := NewRecorder(tc.serverName)
			rec.RecordResourceRead(tc.request, tc.result, tc.err, fixedTime)

			history := rec.GetHistory()
			require.Len(t, history.ResourceReads, 1)

			read := history.ResourceReads[0]
			assert.Equal(t, tc.serverName, read.ServerName)
			assert.Equal(t, fixedTime, read.Timestamp)
			assert.Equal(t, tc.expectedSuccess, read.Success)
			assert.Equal(t, tc.expectedError, read.Error)
			assert.Equal(t, tc.expectedURI, read.URI)
			assert.Equal(t, tc.request, read.Request)
			assert.Equal(t, tc.result, read.Result)
		})
	}
}

func TestRecorderRecordPromptGet(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	tests := map[string]struct {
		serverName      string
		request         *mcp.GetPromptRequest
		result          *mcp.GetPromptResult
		err             error
		expectedSuccess bool
		expectedError   string
		expectedName    string
	}{
		"successful get": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.GetPromptParams]{
				Params: &mcp.GetPromptParams{Name: "greeting-prompt"},
			},
			result:          &mcp.GetPromptResult{},
			err:             nil,
			expectedSuccess: true,
			expectedError:   "",
			expectedName:    "greeting-prompt",
		},
		"failed get": {
			serverName: "test-server",
			request: &mcp.ServerRequest[*mcp.GetPromptParams]{
				Params: &mcp.GetPromptParams{Name: "unknown-prompt"},
			},
			result:          nil,
			err:             errors.New("prompt not found"),
			expectedSuccess: false,
			expectedError:   "prompt not found",
			expectedName:    "unknown-prompt",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			rec := NewRecorder(tc.serverName)
			rec.RecordPromptGet(tc.request, tc.result, tc.err, fixedTime)

			history := rec.GetHistory()
			require.Len(t, history.PromptGets, 1)

			prompt := history.PromptGets[0]
			assert.Equal(t, tc.serverName, prompt.ServerName)
			assert.Equal(t, fixedTime, prompt.Timestamp)
			assert.Equal(t, tc.expectedSuccess, prompt.Success)
			assert.Equal(t, tc.expectedError, prompt.Error)
			assert.Equal(t, tc.expectedName, prompt.Name)
			assert.Equal(t, tc.request, prompt.Request)
			assert.Equal(t, tc.result, prompt.Result)
		})
	}
}

func TestRecorderGetHistory(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	t.Run("returns empty history initially", func(t *testing.T) {
		rec := NewRecorder("test-server")
		history := rec.GetHistory()

		assert.NotNil(t, history.ToolCalls)
		assert.NotNil(t, history.ResourceReads)
		assert.NotNil(t, history.PromptGets)
		assert.Empty(t, history.ToolCalls)
		assert.Empty(t, history.ResourceReads)
		assert.Empty(t, history.PromptGets)
	})

	t.Run("returns recorded calls across all types", func(t *testing.T) {
		rec := NewRecorder("test-server")

		// Record 2 tool calls
		for i := 0; i < 2; i++ {
			req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
				Params: &mcp.CallToolParamsRaw{Name: "tool"},
			}
			rec.RecordToolCall(req, nil, nil, fixedTime)
		}

		// Record 1 resource read
		resReq := &mcp.ServerRequest[*mcp.ReadResourceParams]{
			Params: &mcp.ReadResourceParams{URI: "file:///test"},
		}
		rec.RecordResourceRead(resReq, nil, nil, fixedTime)

		// Record 1 prompt get
		promptReq := &mcp.ServerRequest[*mcp.GetPromptParams]{
			Params: &mcp.GetPromptParams{Name: "prompt"},
		}
		rec.RecordPromptGet(promptReq, nil, nil, fixedTime)

		history := rec.GetHistory()
		assert.Len(t, history.ToolCalls, 2)
		assert.Len(t, history.ResourceReads, 1)
		assert.Len(t, history.PromptGets, 1)
	})

	t.Run("returns value copy not reference", func(t *testing.T) {
		rec := NewRecorder("test-server")

		req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
			Params: &mcp.CallToolParamsRaw{Name: "tool"},
		}
		rec.RecordToolCall(req, nil, nil, fixedTime)

		history1 := rec.GetHistory()
		assert.Len(t, history1.ToolCalls, 1)

		// Modify the returned history
		history1.ToolCalls = append(history1.ToolCalls, &ToolCall{})

		// Get history again - should still have 1 entry
		history2 := rec.GetHistory()
		assert.Len(t, history2.ToolCalls, 1)
	})
}

func TestRecorderConcurrency(t *testing.T) {
	fixedTime := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)

	t.Run("concurrent recording is safe", func(t *testing.T) {
		rec := NewRecorder("test-server")
		var wg sync.WaitGroup

		numGoroutines := 10
		callsPerGoroutine := 100

		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < callsPerGoroutine; j++ {
					req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
						Params: &mcp.CallToolParamsRaw{Name: "tool"},
					}
					rec.RecordToolCall(req, nil, nil, fixedTime)
				}
			}()
		}

		wg.Wait()

		history := rec.GetHistory()
		assert.Len(t, history.ToolCalls, numGoroutines*callsPerGoroutine)
	})

	t.Run("concurrent read and write is safe", func(t *testing.T) {
		rec := NewRecorder("test-server")
		var wg sync.WaitGroup

		numWriters := 5
		numReaders := 5
		callsPerWriter := 50

		// Start writers
		wg.Add(numWriters)
		for i := 0; i < numWriters; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < callsPerWriter; j++ {
					req := &mcp.ServerRequest[*mcp.CallToolParamsRaw]{
						Params: &mcp.CallToolParamsRaw{Name: "tool"},
					}
					rec.RecordToolCall(req, nil, nil, fixedTime)
				}
			}()
		}

		// Start readers
		wg.Add(numReaders)
		for i := 0; i < numReaders; i++ {
			go func() {
				defer wg.Done()
				for j := 0; j < callsPerWriter; j++ {
					_ = rec.GetHistory()
				}
			}()
		}

		wg.Wait()

		history := rec.GetHistory()
		assert.Len(t, history.ToolCalls, numWriters*callsPerWriter)
	})
}
