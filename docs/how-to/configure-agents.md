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

Uses Anthropic's Claude Code CLI:

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

ACP (Agent Control Protocol) mode gives structured access to agent data including tool calls, thinking, and token estimates. Any agent that implements the ACP protocol can be used.

1. Install an ACP adapter (example: Claude Code):
```bash
npm install -g @zed-industries/claude-code-acp
```

2. Create an agent YAML:
```yaml
kind: Agent
metadata:
  name: "claude-code-acp"
acp:
  cmd: "claude-code-acp"
```

3. Reference it in your eval config:
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
  name: "claude-custom"
builtin:
  type: "claude-code"
commands:
  useVirtualHome: true  # Override just this setting
```
