# mcpchecker

🧪 Test your MCP servers by having AI agents complete real tasks.

## Why MCPChecker?

You've built an MCP server with tools. It works. But:
- **Is your tool description clear enough for an LLM to discover it?**
- **Can an AI agent actually use your tool correctly?**
- **Does your tool handle edge cases properly?**

MCPChecker helps you test these questions automatically by:
1. Running real AI agents (like Claude Code) against your tools
2. Verifying agents can discover and use your tools correctly
3. Testing edge cases and error handling
4. Ensuring tool descriptions are clear and actionable

Think of it as integration testing for AI tool use.

## Installation

### Homebrew (macOS)

```bash
brew tap mcpchecker/mcpchecker
brew install mcpchecker
```

### Fedora / RHEL (dnf)

```bash
# x86_64 (replace VERSION, e.g., 1.0.0)
sudo dnf install https://github.com/mcpchecker/mcpchecker/releases/download/vVERSION/mcpchecker_VERSION_linux_amd64.rpm

# arm64
sudo dnf install https://github.com/mcpchecker/mcpchecker/releases/download/vVERSION/mcpchecker_VERSION_linux_arm64.rpm
```

### Debian / Ubuntu (apt)

```bash
# x86_64 (replace VERSION, e.g., 1.0.0)
curl -LO https://github.com/mcpchecker/mcpchecker/releases/download/vVERSION/mcpchecker_VERSION_linux_amd64.deb
sudo apt install ./mcpchecker_VERSION_linux_amd64.deb

# arm64
curl -LO https://github.com/mcpchecker/mcpchecker/releases/download/vVERSION/mcpchecker_VERSION_linux_arm64.deb
sudo apt install ./mcpchecker_VERSION_linux_arm64.deb
```

### Manual Download

Download the latest release from [GitHub Releases](https://github.com/mcpchecker/mcpchecker/releases):

```bash
# Linux (amd64)
curl -L -o mcpchecker.zip https://github.com/mcpchecker/mcpchecker/releases/latest/download/mcpchecker-linux-amd64.zip
unzip mcpchecker.zip
sudo mv mcpchecker /usr/local/bin/

# macOS (Apple Silicon)
curl -L -o mcpchecker.zip https://github.com/mcpchecker/mcpchecker/releases/latest/download/mcpchecker-darwin-arm64.zip
unzip mcpchecker.zip
sudo mv mcpchecker /usr/local/bin/

# macOS (Intel)
curl -L -o mcpchecker.zip https://github.com/mcpchecker/mcpchecker/releases/latest/download/mcpchecker-darwin-amd64.zip
unzip mcpchecker.zip
sudo mv mcpchecker /usr/local/bin/
```

### Verify Installation

```bash
mcpchecker --version
```

See [Quickstarts](https://github.com/mcpchecker/quickstarts) for step-by-step tutorials.

## What It Does

mcpchecker validates MCP servers by:
1. 🔧 Running setup scripts (e.g., create test namespace)
2. 🤖 Giving an AI agent a task prompt (e.g., "create a nginx pod")
3. 📝 Recording which MCP tools the agent uses
4. ✅ Verifying the task succeeded via scripts OR LLM judge (e.g., pod is running, or response contains expected content)
5. 🔍 Checking assertions (e.g., did agent call `pods_create`?)
6. 🧹 Running cleanup scripts

If agents successfully complete tasks using your MCP server, your tools are well-designed.

## Quick Start

**For first-time users**: Check out the [Quickstarts](https://github.com/mcpchecker/quickstarts) for hands-on tutorials.

**For development** (building from source):

```bash
# Build
go build -o mcpchecker ./cmd/mcpchecker

# Run the example (requires Kubernetes cluster + MCP server)
./mcpchecker check examples/kubernetes/eval.yaml
```

The tool will:
- Display progress in real-time
- Save results to `mcpchecker-<name>-out.json`
- Show pass/fail summary

## Example Setup

**eval.yaml** - Main config:
```yaml
kind: Eval
metadata:
  name: "kubernetes-test"
config:
  # Option 1: Inline builtin agent (no separate file needed)
  agent:
    type: "builtin.claude-code"

  # Option 2: OpenAI-compatible builtin agent
  # agent:
  #   type: "builtin.openai-agent"
  #   model: "gpt-4"

  # Option 3: Reference a custom agent file
  # agent:
  #   type: "file"
  #   path: agent.yaml

  mcpConfigFile: mcp-config.yaml  # Your MCP server config
  llmJudge:                        # Optional: LLM judge for semantic verification
    env:
      baseUrlKey: JUDGE_BASE_URL   # Env var name for LLM API base URL
      apiKeyKey: JUDGE_API_KEY     # Env var name for LLM API key
      modelNameKey: JUDGE_MODEL_NAME # Env var name for model name
  taskSets:
    - path: tasks/create-pod.yaml
      assertions:
        toolsUsed:
          - server: kubernetes
            toolPattern: "pods_.*"  # Agent must use pod-related tools
        minToolCalls: 1
        maxToolCalls: 10
    # Or use globs with optional label filtering:
    # - glob: tasks/**/*.yaml
    #   labelSelector:
    #     suite: kubernetes  # Only run tasks with label suite=kubernetes
    #     category: basic    # AND label category=basic (AND logic)
```

**mcp-config.yaml** - MCP server to test:
```yaml
mcpServers:
  kubernetes:
    type: http
    url: http://localhost:8080/mcp
    enableAllTools: true
```

**agent.yaml** - AI agent configuration:
```yaml
kind: Agent
metadata:
  name: "claude-code"
builtin:
  type: "claude-code"  # Use built-in Claude Code configuration
```

Or with OpenAI-compatible agents:
```yaml
kind: Agent
metadata:
  name: "my-agent"
builtin:
  type: "openai-agent"
  model: "gpt-4"
# Set these environment variables:
# export MODEL_BASE_URL="https://api.openai.com/v1"
# export MODEL_KEY="sk-..."
```

For custom configurations, specify the `commands` section manually (see "Agent Configuration" below).

**tasks/create-pod.yaml** - Test task:
```yaml
kind: Task
metadata:
  name: "create-nginx-pod"
  difficulty: easy
  labels:
    suite: kubernetes
    category: basic
steps:
  setup:
    file: setup.sh      # Creates test namespace
  verify:
    file: verify.sh     # Script-based: Checks pod is running
    # OR use LLM judge (requires llmJudge config in eval.yaml):
    # contains: "pod is running"  # Semantic check: response contains this text
    # exact: "The pod web-server is running"  # Semantic check: exact match
  cleanup:
    file: cleanup.sh    # Deletes pod
  prompt:
    inline: Create a nginx pod named web-server in the test-namespace
```

Note: You must choose either script-based verification (`file` or `inline`) OR LLM judge verification (`contains` or `exact`), not both.

## Task Organization and Filtering

### Using Labels

Tasks can include labels for categorization and filtering:

```yaml
kind: Task
metadata:
  name: "create-pod"
  difficulty: easy
  labels:
    suite: kubernetes
    category: basic
    requires: cluster
```

### Filtering with Label Selectors

Use `labelSelector` in eval configs to filter tasks:

```yaml
# Run only kubernetes tasks
taskSets:
  - glob: tasks/**/*.yaml
    labelSelector:
      suite: kubernetes

# Run only kiali tasks that require istio
taskSets:
  - glob: tasks/**/*.yaml
    labelSelector:
      suite: kiali
      requires: istio

# Multiple task sets with different filters
taskSets:
  - glob: tasks/kubernetes/*/*.yaml
    labelSelector:
      suite: kubernetes
    assertions:
      minToolCalls: 1
      maxToolCalls: 20
  - glob: tasks/kiali/*/*.yaml
    labelSelector:
      suite: kiali
    assertions:
      minToolCalls: 1
      maxToolCalls: 40
```

**Label Selector Logic:**
- All labels in the selector must match (AND logic)
- If `labelSelector` is omitted or empty, all tasks matched by the glob/path are included
- Tasks without labels will not match any non-empty label selector
- Combines with glob/path patterns - both must match for a task to be included

**Best Practices:**
- Use consistent label keys across your task suite (`suite`, `category`, `requires`, etc.)
- Combine directory structure with labels for robust organization
- Use globs for path-based filtering, labels for semantic filtering

## Assertions

Validate agent behavior:

```yaml
assertions:
  # Must call these tools
  toolsUsed:
    - server: kubernetes
      tool: pods_create              # Exact tool name
    - server: kubernetes
      toolPattern: "pods_.*"         # Regex pattern

  # Must call at least one of these
  requireAny:
    - server: kubernetes
      tool: pods_create

  # Must NOT call these
  toolsNotUsed:
    - server: kubernetes
      tool: namespaces_delete

  # Call limits
  minToolCalls: 1
  maxToolCalls: 10

  # Resource access
  resourcesRead:
    - server: filesystem
      uriPattern: "/data/.*\\.json$"
  resourcesNotRead:
    - server: filesystem
      uri: /etc/secrets/password

  # Prompt usage
  promptsUsed:
    - server: templates
      prompt: deployment-template

  # Call order (can have other calls between)
  callOrder:
    - type: tool
      server: kubernetes
      name: namespaces_create
    - type: tool
      server: kubernetes
      name: pods_create

  # No duplicate calls
  noDuplicateCalls: true
```

## Test Scripts

Scripts return exit 0 for success, non-zero for failure:

**setup.sh** - Prepare environment:
```bash
#!/usr/bin/env bash
kubectl create namespace test-ns
```

**verify.sh** - Check task succeeded:
```bash
#!/usr/bin/env bash
kubectl wait --for=condition=Ready pod/web-server -n test-ns --timeout=120s
```

**cleanup.sh** - Remove resources:
```bash
#!/usr/bin/env bash
kubectl delete pod web-server -n test-ns
```

Or use inline scripts in the task YAML:
```yaml
steps:
  setup:
    inline: |-
      #!/usr/bin/env bash
      kubectl create namespace test-ns
```

## LLM Judge Verification

Instead of script-based verification, you can use an LLM judge to semantically evaluate agent responses. This is useful when:
- You want to check if the agent's response contains specific information (semantic matching)
- The expected output format may vary but the meaning should be consistent
- You're testing tasks where the agent provides text responses rather than performing actions

### Configuration

First, configure the LLM judge in your `eval.yaml`:

```yaml
config:
  llmJudge:
    env:
      baseUrlKey: JUDGE_BASE_URL    # Environment variable for LLM API base URL
      apiKeyKey: JUDGE_API_KEY      # Environment variable for LLM API key
      modelNameKey: JUDGE_MODEL_NAME # Environment variable for model name
```

Set the required environment variables before running:
```bash
export JUDGE_BASE_URL="https://api.openai.com/v1"
export JUDGE_API_KEY="sk-..."
export JUDGE_MODEL_NAME="gpt-4o"
```

**Note**: The LLM judge currently only supports OpenAI-compatible APIs (APIs that follow the OpenAI API format). The implementation uses the OpenAI Go SDK with a configurable base URL, so you can use any OpenAI-compatible endpoint, but APIs with different formats are not supported.

### Evaluation Modes

The LLM judge supports two evaluation modes:

**CONTAINS mode** (`verify.contains`):
- Checks if the agent's response semantically contains all core information from the reference answer
- Extra, correct, and non-contradictory information is acceptable
- Format and phrasing differences are ignored (semantic matching)
- Use when you want to verify the response includes specific information

**EXACT mode** (`verify.exact`):
- Checks if the agent's response is semantically equivalent to the reference answer
- Simple rephrasing is acceptable (e.g., "Paris is the capital" vs "The capital is Paris")
- Adding or omitting information will fail
- Use when you need precise semantic equivalence

**Note**: Both modes use the same LLM-based semantic evaluation approach. The difference is only in the system prompt instructions given to the judge LLM. See [`pkg/llmjudge/prompts.go`](pkg/llmjudge/prompts.go) for the implementation details.

### Usage in Tasks

In your task YAML, use `verify.contains` or `verify.exact` instead of `verify.file` or `verify.inline`:

```yaml
steps:
  verify:
    contains: "mysql:8.0.36"  # Response must contain this information
```

```yaml
steps:
  verify:
    exact: "The pod web-server is running in namespace test-ns"  # Response must match exactly (semantically)
```

**Important**: You cannot use both script-based verification and LLM judge verification in the same task. Choose one method:
- Script-based: `verify.file` or `verify.inline` (runs a script that returns exit code 0 for success)
- LLM judge: `verify.contains` or `verify.exact` (semantically evaluates the agent's text response)

## Results

Pass/fail means:

**✅ Pass** → Your MCP server is well-designed
- Tools are discoverable
- Descriptions are clear
- Schemas work
- Implementation is correct

**❌ Fail** → Needs improvement
- Tool descriptions unclear
- Schema too complex
- Missing functionality
- Implementation bugs

## Output

Results saved to `mcpchecker-<eval-name>-out.json`:

```json
{
  "taskName": "create-nginx-pod",
  "taskPassed": true,
  "allAssertionsPassed": true,
  "assertionResults": {
    "toolsUsed": { "passed": true },
    "minToolCalls": { "passed": true }
  },
  "callHistory": {
    "toolCalls": [
      {
        "serverName": "kubernetes",
        "toolName": "pods_create",
        "timestamp": "2025-01-15T10:30:00Z"
      }
    ]
  }
}
```

## Agent Configuration

### Inline vs File-based Configuration

You can configure agents in two ways:

1. **Inline builtin agent** (recommended for simple setups):
```yaml
kind: Eval
config:
  agent:
    type: "builtin.claude-code"
```

2. **Custom agent file**:
```yaml
kind: Eval
config:
  agent:
    type: "file"
    path: agent.yaml
```

Use inline configuration for simple setups with built-in agents. Use a separate file when you need custom commands or want to reuse the same agent across multiple evals.

### Built-in Agent Types

mcpchecker provides built-in configurations for popular AI agents to eliminate boilerplate:

**Claude Code**:
```yaml
kind: Eval
config:
  agent:
    type: "builtin.claude-code"
```

**OpenAI-compatible agents**:
```yaml
kind: Eval
config:
  agent:
    type: "builtin.openai-agent"
    model: "gpt-4"  # or any OpenAI-compatible model
```

Set environment variables for API access:
```bash
# Generic environment variables used by all OpenAI-compatible models
export MODEL_BASE_URL="https://api.openai.com/v1"
export MODEL_KEY="sk-..."

# For other providers (e.g., granite, custom endpoints):
# export MODEL_BASE_URL="https://your-endpoint/v1"
# export MODEL_KEY="your-key"
```

### Available Built-in Types

- `claude-code` - Anthropic's Claude Code CLI
- `openai-agent` - OpenAI-compatible agents using direct API calls (requires model)

### ACP Mode

ACP (Agent Control Protocol) mode provides structured access to agent data including tool calls, thinking, and token estimates. Any agent that implements the ACP protocol can be used.

**1. Install an ACP adapter** (example: Claude Code):
```bash
npm install -g @zed-industries/claude-code-acp
```

**2. Create an agent-acp.yaml:**
```yaml
kind: Agent
metadata:
  name: "claude-code-acp"
acp:
  cmd: "claude-code-acp"
```

**3. Reference it in eval.yaml:**
```yaml
kind: Eval
metadata:
  name: "kubernetes-basic-operations"
config:
  agent:
    type: "file"
    path: agent-acp.yaml
  # ...
```

### Custom Agent Configuration

For custom setups, specify the `commands` section:

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

### Overriding Built-in Defaults

You can use a built-in type and override specific settings:

```yaml
kind: Agent
metadata:
  name: "claude-custom"
builtin:
  type: "claude-code"
commands:
  useVirtualHome: true  # Override just this setting
```

## CLI Commands

### `mcpchecker eval`
Run evaluations against your MCP server:
```bash
mcpchecker eval examples/kubernetes/eval.yaml
```

### `mcpchecker summary`
Display a summary of evaluation results:
```bash
mcpchecker summary results.json                    # Human-readable text
mcpchecker summary results.json --output json      # JSON output
mcpchecker summary results.json --github-output    # GitHub Actions format
mcpchecker summary results.json --task task-name   # Filter by task
```

### `mcpchecker verify`
Verify that results meet minimum pass rate thresholds (useful for CI):
```bash
mcpchecker verify results.json --task 0.8 --assertion 0.9
```
Exits with code 0 if thresholds are met, code 1 otherwise.

### `mcpchecker diff`
Compare two evaluation runs (e.g., main vs PR):
```bash
mcpchecker diff --base results-main.json --current results-pr.json
mcpchecker diff --base results-main.json --current results-pr.json --output markdown
```
Shows regressions, improvements, new tasks, and removed tasks.

### `mcpchecker view`
View detailed results for a specific task:
```bash
mcpchecker view results.json --task task-name
```

## How It Works

The tool creates an MCP proxy that sits between the AI agent and your MCP server:

```
AI Agent → MCP Proxy (recording) → Your MCP Server
```

Everything gets recorded:
- Which tools were called
- What arguments were passed
- When calls happened
- What responses came back

Then assertions validate the recorded behavior matches your expectations.
