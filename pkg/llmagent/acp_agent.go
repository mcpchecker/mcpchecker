package llmagent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"charm.land/fantasy"
	"github.com/coder/acp-go-sdk"
)

type AcpAgent interface {
	RunACP(ctx context.Context, in io.Reader, out io.Writer) error
}

type acpAgent struct {
	model        fantasy.LanguageModel
	systemPrompt string
	conn         *acp.AgentSideConnection
	mu           sync.Mutex
	sessions     map[acp.SessionId]*acpSession
}

var _ acp.Agent = &acpAgent{}

type acpSession struct {
	mu            sync.Mutex
	ctx           context.Context
	sessionCancel context.CancelFunc
	promptCancel  context.CancelFunc
	promptGen     uint64
	mcpClients    []McpClient
}

func New(ctx context.Context, cfg Config) (AcpAgent, error) {
	providerName, modelID, err := cfg.ParseModel()
	if err != nil {
		return nil, err
	}

	provider, err := ResolveProvider(providerName)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider %q: %w", providerName, err)
	}

	model, err := provider.LanguageModel(ctx, modelID)
	if err != nil {
		return nil, fmt.Errorf("failed to create language model %q: %w", modelID, err)
	}

	return &acpAgent{
		model:        model,
		systemPrompt: cfg.SystemPrompt,
		sessions:     make(map[acp.SessionId]*acpSession),
	}, nil
}

func (a *acpAgent) RunACP(ctx context.Context, in io.Reader, out io.Writer) error {
	conn := acp.NewAgentSideConnection(a, out, in)
	a.conn = conn

	defer a.cleanupAllSessions()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-conn.Done():
		return nil
	}
}

// cleanupAllSessions closes all active sessions and their MCP clients.
// It atomically drains the session map to avoid double-cleanup races with Cancel.
func (a *acpAgent) cleanupAllSessions() {
	a.mu.Lock()
	sessions := a.sessions
	a.sessions = make(map[acp.SessionId]*acpSession)
	a.mu.Unlock()

	for _, s := range sessions {
		s.cleanup()
	}
}

func (a *acpAgent) Initialize(_ context.Context, _ acp.InitializeRequest) (acp.InitializeResponse, error) {
	return acp.InitializeResponse{
		ProtocolVersion: acp.ProtocolVersionNumber,
		AgentCapabilities: acp.AgentCapabilities{
			LoadSession: false, // TODO: do we want to support this for debugging in the future?
			McpCapabilities: acp.McpCapabilities{
				Http: true,
			},
		},
	}, nil
}

func (a *acpAgent) NewSession(ctx context.Context, params acp.NewSessionRequest) (acp.NewSessionResponse, error) {
	sessionID := acp.SessionId(randomID("sess"))

	sessionCtx, sessionCancel := context.WithCancel(context.Background())

	mcpClients := make([]McpClient, 0, len(params.McpServers))
	for _, srv := range params.McpServers {
		if srv.Http == nil {
			// currently we only support http servers (as server runs through mcpproxy)
			// TODO:maybe revisit this in the future
			continue
		}
		hdrs := make(http.Header, len(srv.Http.Headers))
		for _, h := range srv.Http.Headers {
			hdrs.Add(h.Name, h.Value)
		}

		client, err := NewMcpClient(ctx, srv.Http.Url, hdrs)
		if err != nil {
			for _, c := range mcpClients {
				c.Close()
			}
			sessionCancel()
			return acp.NewSessionResponse{}, fmt.Errorf("failed to create MCP client for %s: %w", srv.Http.Name, err)
		}

		mcpClients = append(mcpClients, client)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.sessions[sessionID] = &acpSession{
		ctx:           sessionCtx,
		sessionCancel: sessionCancel,
		mcpClients:    mcpClients,
	}

	return acp.NewSessionResponse{SessionId: sessionID}, nil
}

func (a *acpAgent) Authenticate(_ context.Context, _ acp.AuthenticateRequest) (acp.AuthenticateResponse, error) {
	return acp.AuthenticateResponse{}, nil
}

func (a *acpAgent) Cancel(_ context.Context, params acp.CancelNotification) error {
	a.mu.Lock()
	s, ok := a.sessions[params.SessionId]
	if ok {
		delete(a.sessions, params.SessionId)
	}
	a.mu.Unlock()

	if s != nil {
		s.cleanup()
	}

	return nil
}

func (a *acpAgent) ListSessions(_ context.Context, _ acp.ListSessionsRequest) (acp.ListSessionsResponse, error) {
	return acp.ListSessionsResponse{}, nil
}

func (a *acpAgent) SetSessionMode(_ context.Context, _ acp.SetSessionModeRequest) (acp.SetSessionModeResponse, error) {
	return acp.SetSessionModeResponse{}, nil
}

func (a *acpAgent) SetSessionConfigOption(_ context.Context, _ acp.SetSessionConfigOptionRequest) (acp.SetSessionConfigOptionResponse, error) {
	return acp.SetSessionConfigOptionResponse{}, nil
}

func (a *acpAgent) Prompt(ctx context.Context, params acp.PromptRequest) (acp.PromptResponse, error) {
	a.mu.Lock()
	s, ok := a.sessions[params.SessionId]
	a.mu.Unlock()

	if !ok {
		return acp.PromptResponse{}, fmt.Errorf("session %s not found", params.SessionId)
	}

	// cancel any previous turn
	s.mu.Lock()
	if s.promptCancel != nil {
		cancelPrev := s.promptCancel
		s.mu.Unlock()
		cancelPrev()
	} else {
		s.mu.Unlock()
	}

	promptCtx, promptCancel := context.WithCancel(s.ctx)
	defer promptCancel()

	s.mu.Lock()
	s.promptGen++
	myGen := s.promptGen
	s.promptCancel = promptCancel
	s.mu.Unlock()

	var promptBuilder strings.Builder
	for _, p := range params.Prompt {
		if p.Text != nil {
			promptBuilder.WriteString(p.Text.Text)
		}
	}

	prompt := promptBuilder.String()

	tools := ToolsFromMcpClients(s.mcpClients, a.toolInterceptorForSession(params.SessionId))

	var opts []fantasy.AgentOption

	if a.systemPrompt != "" {
		opts = append(opts, fantasy.WithSystemPrompt(a.systemPrompt))
	}
	if len(tools) > 0 {
		opts = append(opts, fantasy.WithTools(tools...))
	}

	agent := fantasy.NewAgent(a.model, opts...)

	result, err := agent.Stream(promptCtx, fantasy.AgentStreamCall{
		Prompt: prompt,
		OnStepFinish: func(step fantasy.StepResult) error {
			text := step.Response.Content.Text()
			if text == "" {
				return nil
			}

			return a.conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update:    acp.UpdateAgentMessageText(text),
			})
		},
		OnToolResult: func(result fantasy.ToolResultContent) error {
			output := ""
			if result.Result != nil {
				if textResult, ok := fantasy.AsToolResultOutputType[fantasy.ToolResultOutputContentText](result.Result); ok {
					output = textResult.Text
				}
			}

			return a.conn.SessionUpdate(promptCtx, acp.SessionNotification{
				SessionId: params.SessionId,
				Update: acp.UpdateToolCall(
					acp.ToolCallId(result.ToolCallID),
					acp.WithUpdateStatus(acp.ToolCallStatusCompleted),
					acp.WithUpdateRawOutput(output),
				),
			})
		},
	})

	// Only clear cancel if it's still ours (another Prompt may have started)
	s.mu.Lock()
	if s.promptGen == myGen {
		s.promptCancel = nil
	}
	s.mu.Unlock()

	if err != nil {
		if promptCtx.Err() != nil {
			return acp.PromptResponse{StopReason: acp.StopReasonCancelled}, nil
		}
		return acp.PromptResponse{}, err
	}

	// Usage is passed via Meta until the ACP SDK adds first-class support for the
	// session usage RFD: https://agentclientprotocol.com/rfds/session-usage
	return acp.PromptResponse{
		StopReason: acp.StopReasonEndTurn,
		Meta: map[string]any{
			"usage": result.TotalUsage,
		},
	}, nil
}

func (a *acpAgent) toolInterceptorForSession(sessionId acp.SessionId) toolInterceptor {
	return func(ctx context.Context, call fantasy.ToolCall) (bool, error) {
		toolId := acp.ToolCallId(call.ID)

		var rawInput any
		if call.Input != "" {
			if err := json.Unmarshal([]byte(call.Input), &rawInput); err != nil {
				rawInput = call.Input
			}
		}

		// notify ACP client of tool call start
		if err := a.conn.SessionUpdate(ctx, acp.SessionNotification{
			SessionId: sessionId,
			Update: acp.StartToolCall(
				toolId,
				call.Name,
				acp.WithStartStatus(acp.ToolCallStatusPending),
				acp.WithStartRawInput(rawInput),
			),
		}); err != nil {
			return false, err
		}

		resp, err := a.conn.RequestPermission(ctx, acp.RequestPermissionRequest{
			SessionId: sessionId,
			ToolCall: acp.ToolCallUpdate{
				ToolCallId: toolId,
				RawInput:   rawInput,
			},
			Options: []acp.PermissionOption{
				{Kind: acp.PermissionOptionKindAllowOnce, Name: "Allow", OptionId: "allow"},
				{Kind: acp.PermissionOptionKindRejectOnce, Name: "Reject", OptionId: "reject"},
			},
		})
		if err != nil {
			return false, err
		}

		if resp.Outcome.Cancelled != nil || resp.Outcome.Selected == nil {
			return false, nil
		}

		return resp.Outcome.Selected.OptionId == "allow", nil
	}
}

func (s *acpSession) cleanup() {
	s.mu.Lock()
	cancelPrompt := s.promptCancel
	s.promptCancel = nil
	s.mu.Unlock()

	if cancelPrompt != nil {
		cancelPrompt()
	}
	if s.sessionCancel != nil {
		s.sessionCancel()
	}
	for _, c := range s.mcpClients {
		c.Close()
	}
}

func randomID(prefix string) string {
	var b [12]byte

	if _, err := io.ReadFull(rand.Reader, b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}

	return prefix + "_" + hex.EncodeToString(b[:])
}
