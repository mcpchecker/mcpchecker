# Configure Agents

mcpchecker needs an AI agent to run tasks against your MCP server. You can use built-in agent types or define your own.

## Inline vs File-based Configuration

There are two ways to configure agents:

**Inline** (recommended for simple setups) -- define the agent directly in your eval config:
```yaml
kind: Eval
config:
  agent:
    type: "builtin.claude-code"
```

**File-based** -- reference a separate agent YAML file:
```yaml
kind: Eval
config:
  agent:
    type: "file"
    path: agent.yaml
```

Use a separate file when you need custom commands or want to reuse the same agent across multiple evals.

## Built-in Agent Types

### Claude Code

Uses Anthropic's Claude Code CLI via the ACP protocol, providing structured output with tool calls, thinking steps, and token usage:

```yaml
kind: Eval
config:
  agent:
    type: "builtin.claude-code"
```

Or as a standalone file:
```yaml
kind: Agent
metadata:
  name: "claude-code"
builtin:
  type: "claude-code"
```

**Prerequisite:** Install the Claude Code ACP adapter:
```bash
npm install -g @agentclientprotocol/claude-agent-acp
```

### LLM Agent

A multi-provider agent that supports OpenAI, Anthropic, Gemini, and any OpenAI-compatible endpoint. Uses the `provider:model-id` format:

```yaml
kind: Eval
config:
  agent:
    type: "builtin.llm-agent"
    model: "openai:gpt-4"
```

Set the appropriate environment variables for your provider:

```bash
# OpenAI
export OPENAI_API_KEY="sk-..."

# Anthropic
export ANTHROPIC_API_KEY="sk-ant-..."

# Anthropic via Vertex AI
export ANTHROPIC_USE_VERTEX=1
export GOOGLE_CLOUD_PROJECT="your-project"
export GOOGLE_CLOUD_LOCATION="us-central1"

# Gemini
export GEMINI_API_KEY="..."

# Gemini via Vertex AI
export GEMINI_USE_VERTEX=1
export GOOGLE_CLOUD_PROJECT="your-project"
export GOOGLE_CLOUD_LOCATION="us-central1"

# Custom OpenAI-compatible endpoints
export OPENAI_BASE_URL="https://your-endpoint/v1"
export OPENAI_API_KEY="your-key"
```

## ACP Mode

ACP (Agent Client Protocol) mode gives structured access to agent data including tool calls, thinking, and token estimates. The `builtin.claude-code` and `builtin.llm-agent` types use ACP by default.

For other agents that implement the ACP protocol, use the `acp` config directly:

```yaml
kind: Agent
metadata:
  name: "my-acp-agent"
acp:
  cmd: "my-acp-binary"
  args:
    - "--verbose"
```

Reference it in your eval config:
```yaml
kind: Eval
config:
  agent:
    type: "file"
    path: agent-acp.yaml
```

## Custom Agent Configuration

For agents not covered by the built-in types, specify the `commands` section directly:

```yaml
kind: Agent
metadata:
  name: "custom-agent"
commands:
  useVirtualHome: false
  argTemplateMcpServer: "--mcp {{ .File }}"
  argTemplateAllowedTools: "{{ .ToolName }}"
  runPrompt: |-
    my-agent --mcp-config {{ .McpServerFileArgs }} --prompt "{{ .Prompt }}"
```

## Overriding Built-in Defaults

You can start from a built-in type and override specific settings:

```yaml
kind: Agent
metadata:
  name: "custom-llm"
builtin:
  type: "llm-agent"
  model: "openai:gpt-4"
commands:
  useVirtualHome: true  # Override just this setting
```

Note: Command overrides only apply to shell-based agents. The `claude-code` builtin uses ACP and does not use the `commands` section.
