package mcpproxy

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Recorder interface {
	RecordToolCall(req *mcp.CallToolRequest, res *mcp.CallToolResult, err error, start time.Time)
	RecordResourceRead(req *mcp.ReadResourceRequest, res *mcp.ReadResourceResult, err error, start time.Time)
	RecordPromptGet(req *mcp.GetPromptRequest, res *mcp.GetPromptResult, err error, start time.Time)
	GetHistory() CallHistory
}

// TokenCount provides token count estimates for a single MCP call.
type TokenCount struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

// NewTokenCount creates a TokenCount with computed TotalTokens.
func NewTokenCount(input, output int64) *TokenCount {
	return &TokenCount{
		InputTokens:  input,
		OutputTokens: output,
		TotalTokens:  input + output,
	}
}

// CallRecord is the base for all MCP interaction types
type CallRecord struct {
	ServerName string    `json:"serverName"`
	Timestamp  time.Time `json:"timestamp"`
	Success    bool      `json:"success"`
	Error      string    `json:"error,omitempty"`
}

type SafeServerRequest[P mcp.Params] struct {
	Session *mcp.ServerSession
	Params  P
	Extra   *SafeRequestExtra
}

func SafeServerRequestFromUnsafe[P mcp.Params](req *mcp.ServerRequest[P]) *SafeServerRequest[P] {
	if req == nil {
		return nil
	}

	res := &SafeServerRequest[P]{
		Session: req.Session,
		Params:  req.Params,
	}

	if req.Extra != nil {
		res.Extra = &SafeRequestExtra{
			TokenInfo: req.Extra.TokenInfo,
		}
	}

	return res
}

type SafeRequestExtra struct {
	TokenInfo *auth.TokenInfo // bearer token info (e.g. from OAuth) if any
}

// ToolCall records a tool invocation
type ToolCall struct {
	CallRecord
	ToolName string               `json:"name"` // this is copied to the top level struct for convenience
	Request  *mcp.CallToolRequest `json:"request,omitempty"`
	Result   *mcp.CallToolResult  `json:"result,omitempty"`
	Tokens   *TokenCount          `json:"tokens,omitempty"`
}

func (c *ToolCall) MarshalJSON() ([]byte, error) {
	type ToolCallAlias ToolCall

	return json.Marshal(&struct {
		*ToolCallAlias
		Request *SafeServerRequest[*mcp.CallToolParamsRaw] `json:"request,omitempty"`
	}{
		ToolCallAlias: (*ToolCallAlias)(c),
		Request:       SafeServerRequestFromUnsafe(c.Request),
	})
}

// ResourceRead records a resource read
type ResourceRead struct {
	CallRecord
	URI     string                   `json:"uri"` // this is copied to the top level struct for convenience
	Request *mcp.ReadResourceRequest `json:"request"`
	Result  *mcp.ReadResourceResult  `json:"result"`
	Tokens  *TokenCount              `json:"tokens,omitempty"`
}

func (r *ResourceRead) MarshalJSON() ([]byte, error) {
	type ResourceReadAlias ResourceRead

	return json.Marshal(&struct {
		*ResourceReadAlias
		Request *SafeServerRequest[*mcp.ReadResourceParams] `json:"request,omitempty"`
	}{
		ResourceReadAlias: (*ResourceReadAlias)(r),
		Request:           SafeServerRequestFromUnsafe(r.Request),
	})
}

// PromptGet records a prompt get
type PromptGet struct {
	CallRecord
	Name    string                `json:"name"` // this is copies to the top level struct for convenience
	Request *mcp.GetPromptRequest `json:"request"`
	Result  *mcp.GetPromptResult  `json:"result"`
	Tokens  *TokenCount           `json:"tokens,omitempty"`
}

func (p *PromptGet) MarshalJSON() ([]byte, error) {
	type PromptGetAlias PromptGet

	return json.Marshal(&struct {
		*PromptGetAlias
		Request *SafeServerRequest[*mcp.GetPromptParams] `json:"request"`
	}{
		PromptGetAlias: (*PromptGetAlias)(p),
		Request:        SafeServerRequestFromUnsafe(p.Request),
	})
}

// CallHistory contains a complete call history for a server
type CallHistory struct {
	ToolCalls     []*ToolCall
	ResourceReads []*ResourceRead
	PromptGets    []*PromptGet
}

type recorder struct {
	serverName string

	mu      sync.RWMutex
	history *CallHistory
}

var _ Recorder = &recorder{}

func NewRecorder(serverName string) Recorder {
	return &recorder{
		serverName: serverName,
		history: &CallHistory{
			ToolCalls:     make([]*ToolCall, 0),
			ResourceReads: make([]*ResourceRead, 0),
			PromptGets:    make([]*PromptGet, 0),
		},
	}
}

func (r *recorder) RecordToolCall(req *mcp.CallToolRequest, res *mcp.CallToolResult, err error, start time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.history.ToolCalls = append(r.history.ToolCalls, &ToolCall{
		CallRecord: CallRecord{
			ServerName: r.serverName,
			Timestamp:  start,
			Success:    err == nil,
			Error:      errorToString(err),
		},
		ToolName: req.Params.Name,
		Request:  req,
		Result:   res,
	})
}

func (r *recorder) RecordResourceRead(req *mcp.ReadResourceRequest, res *mcp.ReadResourceResult, err error, start time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.history.ResourceReads = append(r.history.ResourceReads, &ResourceRead{
		CallRecord: CallRecord{
			ServerName: r.serverName,
			Timestamp:  start,
			Success:    err == nil,
			Error:      errorToString(err),
		},
		URI:     req.Params.URI,
		Request: req,
		Result:  res,
	})
}

func (r *recorder) RecordPromptGet(req *mcp.GetPromptRequest, res *mcp.GetPromptResult, err error, start time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.history.PromptGets = append(r.history.PromptGets, &PromptGet{
		CallRecord: CallRecord{
			ServerName: r.serverName,
			Timestamp:  start,
			Success:    err == nil,
			Error:      errorToString(err),
		},
		Name:    req.Params.Name,
		Request: req,
		Result:  res,
	})
}

func (r *recorder) GetHistory() CallHistory {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return *r.history
}

func errorToString(err error) string {
	if err == nil {
		return ""
	}

	return err.Error()
}
