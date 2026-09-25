package agent

import "fmt"

// LoadWithBuiltins loads an agent spec from a file and merges with builtin defaults if specified
func LoadWithBuiltins(yamlPath string) (*AgentSpec, error) {
	// Load YAML
	spec, err := FromFile(yamlPath)
	if err != nil {
		return nil, err
	}

	// If no builtin reference, return as-is
	if spec.Builtin == nil {
		return spec, nil
	}

	defaults, err := loadBuiltin(spec.Builtin.Type, spec.Builtin.Model)
	if err != nil {
		return nil, err
	}

	// Merge: YAML overrides defaults
	merged := mergeAgentSpecs(defaults, spec)

	return merged, nil
}

func loadBuiltin(builtinType, builtinModel string) (*AgentSpec, error) {
	// Get builtin agent
	builtinAgent, ok := GetBuiltinType(builtinType)
	if !ok {
		return nil, fmt.Errorf("unknown builtin type: %s", builtinType)
	}

	// Validate model requirement
	if builtinAgent.RequiresModel() && builtinModel == "" {
		return nil, fmt.Errorf("builtin type '%s' requires a model to be specified", builtinType)
	}

	// Validate environment
	if err := builtinAgent.ValidateEnvironment(); err != nil {
		return nil, fmt.Errorf("builtin type '%s' environment validation failed: %w", builtinType, err)
	}

	// Get defaults from builtin
	defaults, err := builtinAgent.GetDefaults(builtinModel)
	if err != nil {
		return nil, fmt.Errorf("failed to get defaults for builtin type '%s': %w", builtinType, err)
	}

	return defaults, nil
}

// mergeAgentSpecs merges two agent specs, with overrides taking precedence over defaults
func mergeAgentSpecs(defaults, overrides *AgentSpec) *AgentSpec {
	result := *defaults

	// Override metadata if specified
	if overrides.Metadata.Name != "" {
		result.Metadata.Name = overrides.Metadata.Name
	}
	if overrides.Metadata.Version != nil {
		result.Metadata.Version = overrides.Metadata.Version
	}

	// Merge builtin configuration: YAML overrides defaults where set
	if overrides.Builtin != nil {
		if result.Builtin == nil {
			// No defaults; take everything from overrides
			result.Builtin = overrides.Builtin
		} else {
			if overrides.Builtin.Type != "" {
				result.Builtin.Type = overrides.Builtin.Type
			}
			if overrides.Builtin.Model != "" {
				result.Builtin.Model = overrides.Builtin.Model
			}
			if overrides.Builtin.UseResponsesAPI != nil {
				result.Builtin.UseResponsesAPI = overrides.Builtin.UseResponsesAPI
			}
			if overrides.Builtin.BaseURL != "" {
				result.Builtin.BaseURL = overrides.Builtin.BaseURL
			}
			if overrides.Builtin.APIKey != "" {
				result.Builtin.APIKey = overrides.Builtin.APIKey
			}
		}
	}

	// Determine if commands were specified in overrides
	// We consider commands specified if any non-zero field is set
	commandsSpecified := overrides.Commands.ArgTemplateMcpServer != "" ||
		overrides.Commands.ArgTemplateAllowedTools != "" ||
		overrides.Commands.RunPrompt != "" ||
		overrides.Commands.AllowedToolsJoinSeparator != nil ||
		overrides.Commands.GetVersion != nil ||
		overrides.Commands.UseVirtualHome != nil

	if commandsSpecified {
		// Override individual command fields if they are non-empty
		if overrides.Commands.ArgTemplateMcpServer != "" {
			result.Commands.ArgTemplateMcpServer = overrides.Commands.ArgTemplateMcpServer
		}
		if overrides.Commands.ArgTemplateAllowedTools != "" {
			result.Commands.ArgTemplateAllowedTools = overrides.Commands.ArgTemplateAllowedTools
		}
		if overrides.Commands.AllowedToolsJoinSeparator != nil {
			result.Commands.AllowedToolsJoinSeparator = overrides.Commands.AllowedToolsJoinSeparator
		}
		if overrides.Commands.RunPrompt != "" {
			result.Commands.RunPrompt = overrides.Commands.RunPrompt
		}
		if overrides.Commands.GetVersion != nil {
			result.Commands.GetVersion = overrides.Commands.GetVersion
		}
		// Only override UseVirtualHome when explicitly set in overrides
		if overrides.Commands.UseVirtualHome != nil {
			result.Commands.UseVirtualHome = overrides.Commands.UseVirtualHome
		}
	}

	return &result
}
