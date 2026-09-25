package agent

import (
	"fmt"
	"strings"
)

const (
	builtinPrefix = "builtin."
)

func ResolveAgentRef(ref *AgentRef) (*AgentSpec, error) {
	if ref == nil {
		return nil, fmt.Errorf("agent ref must not be nil")
	}

	var agentSpec *AgentSpec
	var err error
	if ref.Type == "file" {
		if ref.Path == "" {
			return nil, fmt.Errorf("path must be specified when agent type is 'file'")
		}
		agentSpec, err = LoadWithBuiltins(ref.Path)
	} else if builtinType, isBuiltin := strings.CutPrefix(ref.Type, builtinPrefix); isBuiltin {
		agentSpec, err = loadBuiltin(builtinType, ref.Model)
	} else {
		return nil, fmt.Errorf("agent type must be either 'file' or 'builtin.X' format, got: %q", ref.Type)
	}

	if err != nil {
		return nil, err
	}

	if agentSpec.Builtin != nil && ref.UseResponsesAPI != nil {
		agentSpec.Builtin.UseResponsesAPI = ref.UseResponsesAPI
	}

	return agentSpec, nil
}
