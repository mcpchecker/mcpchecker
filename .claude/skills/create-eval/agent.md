# Agent Configuration Reference

Agent configuration files define how to run AI agents during evaluations, including how to pass MCP server configs and prompts.

## Built-in Agents (Recommended)

Built-in agents use the ACP (Agent Client Protocol) for structured output including tool calls, thinking steps, and token usage.

### Inline Configuration

Reference a builtin agent directly in your eval config:

```yaml
kind: Eval
config:
  agent:
    type: "builtin.claude-code"
```

### File-based Configuration

Or as a standalone agent YAML:

```yaml
kind: Agent
metadata:
  name: "claude-code"
builtin:
  type: "claude-code"
```

### Available Built-in Types

| Type | Description | Requires Model |
|------|-------------|---------------|
| `claude-code` | Anthropic's Claude Code CLI via ACP | No |
| `llm-agent` | Multi-provider LLM agent (OpenAI, Anthropic, Gemini, etc.) | Yes |

### LLM Agent Example

```yaml
kind: Eval
config:
  agent:
    type: "builtin.llm-agent"
    model: "openai:gpt-4"
```

## Custom ACP Agents

For agents that implement the ACP protocol directly:

```yaml
kind: Agent
metadata:
  name: "my-acp-agent"
acp:
  cmd: "my-acp-binary"
  args:
    - "--verbose"
```

## Custom Shell Agents

For agents that don't implement ACP, use the `commands` section to define shell-based execution.

### Agent YAML Structure

```yaml
kind: Agent
metadata:
  name: "agent-name"
  version: "1.0.0"  # optional
commands:
  useVirtualHome: false
  argTemplateMcpServer: "--mcp-config {{ .File }}"
  argTemplateAllowedTools: "mcp__{{ .ServerName }}__{{ .ToolName }}"
  allowedToolsJoinSeparator: " "  # optional
  runPrompt: |
    my-agent {{ .McpServerFileArgs }} --print "{{ .Prompt }}"
  getVersion: "my-agent --version"  # optional
```

### Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | Must be `"Agent"` |
| `metadata` | object | Yes | Agent metadata (see below) |
| `commands` | object | Yes (for shell agents) | Command configuration (see below) |

### metadata Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Name of the agent |
| `version` | string | No | Agent version (overridden by `getVersion` if present) |

### commands Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `useVirtualHome` | boolean | No | Create isolated `$HOME` for agent (default: `false`) |
| `argTemplateMcpServer` | string | Yes | Template for MCP config file args (see below) |
| `argTemplateAllowedTools` | string | Yes | Template for allowed tool args (see below) |
| `allowedToolsJoinSeparator` | string | No | Separator for joining tools (default: `" "`) |
| `runPrompt` | string | Yes | Full command template to run agent (see below) |
| `getVersion` | string | No | Command to get agent version dynamically |

### Template Variables

#### argTemplateMcpServer

Applied to each MCP server config file.

| Variable | Description |
|----------|-------------|
| `{{ .File }}` | Path to the MCP server config file |

**Example**:
```yaml
argTemplateMcpServer: "--mcp-config {{ .File }}"
```
→ Produces: `--mcp-config /tmp/mcp-config-123.json`

#### argTemplateAllowedTools

Applied to each allowed tool.

| Variable | Description |
|----------|-------------|
| `{{ .ServerName }}` | Name of the MCP server |
| `{{ .ToolName }}` | Name of the tool |

**Example**:
```yaml
argTemplateAllowedTools: "mcp__{{ .ServerName }}__{{ .ToolName }}"
```
→ Produces: `mcp__kubernetes__pods_list mcp__kubernetes__pods_create`

#### runPrompt

The complete command to execute the agent.

| Variable | Description |
|----------|-------------|
| `{{ .Prompt }}` | The task prompt text |
| `{{ .McpServerFileArgs }}` | All MCP server file arguments (space-separated) |
| `{{ .AllowedToolArgs }}` | All allowed tool arguments (joined by separator) |

### Custom Agent Example

```yaml
kind: Agent
metadata:
  name: "custom-agent"
  version: "2.0.0"
commands:
  useVirtualHome: true
  argTemplateMcpServer: "--config {{ .File }}"
  argTemplateAllowedTools: "--allow {{ .ServerName }}::{{ .ToolName }}"
  allowedToolsJoinSeparator: " "
  runPrompt: |
    my-agent {{ .McpServerFileArgs }} {{ .AllowedToolArgs }} --task "{{ .Prompt }}"
```

### How Templates Work

When running a shell agent:

1. **Format MCP server args**: Apply `argTemplateMcpServer` to each config file
2. **Format tool args**: Apply `argTemplateAllowedTools` to each allowed tool
3. **Join tools**: Combine tool args using `allowedToolsJoinSeparator`
4. **Build command**: Apply `runPrompt` with all variables
5. **Execute**: Run via `$SHELL -c "command"`
