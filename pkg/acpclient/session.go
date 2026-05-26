package acpclient

import (
	"context"
	"strings"
	"sync"

	"github.com/coder/acp-go-sdk"
	"github.com/mcpchecker/mcpchecker/pkg/mcpproxy"
)

type session struct {
	mu               sync.Mutex
	cwd              string // working directory for this session, used by ReadTextFile
	updates          []acp.SessionUpdate // track all the updates in a json serializable way for future analysis
	toolCallStatuses map[acp.ToolCallId]*acp.SessionToolCallUpdate
	mcpServers       mcpproxy.ServerManager
}

func NewSession(mcpServers mcpproxy.ServerManager, cwd string) *session {
	return &session{
		cwd:              cwd,
		updates:          make([]acp.SessionUpdate, 0),
		toolCallStatuses: make(map[acp.ToolCallId]*acp.SessionToolCallUpdate),
		mcpServers:       mcpServers,
	}
}

func (s *session) recordPermissionToolCall(call acp.ToolCallUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.toolCallStatusUpdateLocked(&acp.SessionToolCallUpdate{
		Meta:       call.Meta,
		Content:    call.Content,
		Kind:       call.Kind,
		Locations:  call.Locations,
		RawInput:   call.RawInput,
		RawOutput:  call.RawOutput,
		Status:     call.Status,
		Title:      call.Title,
		ToolCallId: call.ToolCallId,
	})
}

func (s *session) isAllowedToolCall(ctx context.Context, call acp.ToolCallUpdate) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	var title string
	if call.Title != nil {
		title = *call.Title
	} else {
		// look up the original update with the tool call id
		curr, ok := s.toolCallStatuses[call.ToolCallId]
		if !ok {
			return false
		}

		if curr.Title == nil {
			return false
		}

		title = *curr.Title
	}

	for _, srv := range s.mcpServers.GetMcpServers() {
		for _, t := range srv.GetAllowedTools(ctx) {
			if t == nil {
				continue
			}

			if toolTitleProbablyMatches(title, t.Title, t.Name, srv.GetName()) {
				return true
			}
		}
	}

	return false
}

func (s *session) update(update acp.SessionUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, update)

	// handle tool call updates
	if update.ToolCall != nil {
		s.toolCallStatusUpdateLocked(&acp.SessionToolCallUpdate{
			Content:       update.ToolCall.Content,
			Kind:          &update.ToolCall.Kind,
			Locations:     update.ToolCall.Locations,
			RawInput:      update.ToolCall.RawInput,
			RawOutput:     update.ToolCall.RawOutput,
			SessionUpdate: update.ToolCall.SessionUpdate,
			Status:        &update.ToolCall.Status,
			Title:         &update.ToolCall.Title,
			ToolCallId:    update.ToolCall.ToolCallId,
		})
	}
	if update.ToolCallUpdate != nil {
		s.toolCallStatusUpdateLocked(update.ToolCallUpdate)
	}
}

// toolCallStatusUpdateLocked updates tool call status. Caller must hold s.mu.
func (s *session) toolCallStatusUpdateLocked(update *acp.SessionToolCallUpdate) {
	call, ok := s.toolCallStatuses[update.ToolCallId]
	if !ok {
		s.toolCallStatuses[update.ToolCallId] = update
		return
	}

	if update.Content != nil {
		call.Content = update.Content
	}

	if update.Kind != nil {
		call.Kind = update.Kind
	}

	if update.Locations != nil {
		call.Locations = update.Locations
	}

	if update.RawInput != nil {
		call.RawInput = update.RawInput
	}

	if update.RawOutput != nil {
		call.RawOutput = update.RawOutput
	}

	if update.Status != nil {
		call.Status = update.Status
	}

	if update.Title != nil {
		call.Title = update.Title
	}
}

// toolTitleProbablyMatches helps find matching tool titles that we should approve
// from the spec, it is not clear how to tell which tool title matches which mcp tool
// Open discussion (without answers currently): https://github.com/orgs/agentclientprotocol/discussions/409
func toolTitleProbablyMatches(acpToolTitle, mcpToolTitle, mcpToolName, mcpServerName string) bool {
	if acpToolTitle == mcpToolTitle {
		return true
	}

	if acpToolTitle == mcpToolName {
		return true
	}

	if (strings.Contains(acpToolTitle, "mcp") || strings.Contains(acpToolTitle, mcpServerName)) &&
		strings.Contains(acpToolTitle, mcpToolName) {
		return true
	}

	return false
}
