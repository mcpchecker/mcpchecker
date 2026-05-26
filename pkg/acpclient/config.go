package acpclient

import (
	"context"
	"io"
)

// Transport provides the I/O streams for ACP communication.
// When set on AcpConfig, the client uses these streams instead of spawning a subprocess.
type Transport interface {
	// Start initializes the transport and returns the streams for communication.
	// stdin is written to by the client (sent to the agent).
	// stdout is read by the client (received from the agent).
	Start(ctx context.Context) (stdin io.Writer, stdout io.Reader, err error)
	// Close shuts down the transport.
	Close(ctx context.Context) error
}

type AcpConfig struct {
	// Cmd and Args are used to spawn a subprocess when Transport is nil.
	Cmd  string   `json:"cmd"`
	Args []string `json:"args"`

	// Transport, when set, provides the I/O streams directly instead of spawning a subprocess.
	// This allows in-memory communication with an agent.
	Transport Transport `json:"-"`
}

// SkillInfo provides skill mounting information for ACP agents.
// Implemented by agent.SkillInfo to avoid import cycles.
type SkillInfo interface {
	GetMountPath() string
	GetSourceDirs() []string
}

// ClientOption configures optional behavior for the ACP client.
type ClientOption func(*clientOptions)

type clientOptions struct {
	skills SkillInfo
}

// WithSkills configures skill mounting for the ACP client.
func WithSkills(skills SkillInfo) ClientOption {
	return func(o *clientOptions) {
		o.skills = skills
	}
}
