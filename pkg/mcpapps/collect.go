package mcpapps

import (
	"context"
	"fmt"
	"mime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Observation intentionally contains no HTML or original tool payload.
type Observation struct {
	Server   string `json:"server"`
	Tool     string `json:"tool"`
	CallID   string `json:"callId"`
	URI      string `json:"uri,omitempty"`
	MIMEType string `json:"mimeType,omitempty"`
	Code     string `json:"code,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

type Collector struct {
	targets      map[[2]string]bool
	sequence     atomic.Uint64
	mu           sync.Mutex
	observations []Observation
	active       sync.WaitGroup
	calls        map[uint64]context.CancelFunc
	nextCall     uint64
	finished     bool
}

func NewCollector(expected []Assertion) *Collector {
	c := &Collector{targets: make(map[[2]string]bool), calls: make(map[uint64]context.CancelFunc)}
	for _, a := range expected {
		c.targets[[2]string{a.Server, a.Tool}] = true
	}
	return c
}

func (c *Collector) Observations() []Observation {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Observation(nil), c.observations...)
}

// BeginCall tracks targeted invocations before they reach the upstream server.
// Finish cancels any unfinished calls and waits for their observations to be recorded.
func (c *Collector) BeginCall(ctx context.Context, server, tool string) (context.Context, func(), bool) {
	if !c.targets[[2]string{server, tool}] {
		return ctx, func() {}, true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.finished {
		return ctx, nil, false
	}
	ctx, cancel := context.WithCancel(ctx)
	c.nextCall++
	id := c.nextCall
	c.calls[id] = cancel
	c.active.Add(1)
	return ctx, func() {
		cancel()
		c.mu.Lock()
		delete(c.calls, id)
		c.mu.Unlock()
		c.active.Done()
	}, true
}

// Finish closes admission before waiting, so no new calls can race the final snapshot.
func (c *Collector) Finish() []Observation {
	c.mu.Lock()
	c.finished = true
	cancels := make([]context.CancelFunc, 0, len(c.calls))
	for _, cancel := range c.calls {
		cancels = append(cancels, cancel)
	}
	c.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	c.active.Wait()
	return c.Observations()
}

// ErrorDetail preserves a bounded, single-line diagnostic without an HTML response body.
func ErrorDetail(err error) string {
	if err == nil {
		return ""
	}
	message := strings.Join(strings.Fields(err.Error()), " ")
	if before, _, found := strings.Cut(message, "<"); found {
		message = strings.TrimSpace(before) + " [HTML omitted]"
	}
	runes := []rune(message)
	if len(runes) > 512 {
		message = string(runes[:509]) + "..."
	}
	return message
}

// Observe reads and validates the referenced resource from the same MCP session.
// It never re-executes the tool or puts HTML in the agent conversation.
func (c *Collector) Observe(ctx context.Context, server string, tool *mcp.Tool, result *mcp.CallToolResult, callErr error, read func(context.Context, *mcp.ReadResourceParams, string) (*mcp.ReadResourceResult, error)) {
	if !c.targets[[2]string{server, tool.Name}] {
		return
	}
	o := Observation{Server: server, Tool: tool.Name, CallID: fmt.Sprintf("app-%d", c.sequence.Add(1))}
	defer func() { c.mu.Lock(); c.observations = append(c.observations, o); c.mu.Unlock() }()
	if callErr != nil || result == nil {
		o.Code = "tool_call_failed"
		o.Detail = ErrorDetail(callErr)
		return
	}
	o.URI = resourceURI(result.Meta)
	if o.URI == "" {
		o.URI = ResourceURI(tool)
	}
	if o.URI == "" {
		o.Code = "missing_ui_resource"
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	// Correlate host reads out of band, never through agent-controlled MCP metadata.
	resource, err := read(ctx, &mcp.ReadResourceParams{URI: o.URI}, o.CallID)
	if err != nil || resource == nil {
		o.Code = "ui_resource_read_failed"
		o.Detail = ErrorDetail(err)
		if err == nil {
			o.Detail = "resource read returned no result"
		}
		return
	}
	if len(resource.Contents) != 1 || resource.Contents[0] == nil {
		o.Code = "expected_one_ui_resource"
		return
	}
	content := resource.Contents[0]
	if content.URI != o.URI {
		o.Code = "ui_resource_uri_mismatch"
		return
	}
	media, params, err := mime.ParseMediaType(content.MIMEType)
	if err != nil || media != "text/html" || params["profile"] != "mcp-app" {
		o.Code = "invalid_ui_mime_type"
		return
	}
	o.MIMEType = content.MIMEType
	html := content.Text
	if html == "" {
		html = string(content.Blob)
	}
	if strings.TrimSpace(html) == "" || len(html) > 1<<20 {
		o.Code = "invalid_ui_resource_size"
		return
	}
}
