# Getting Started

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

## Quick Start

Once installed, you need three things to run an evaluation:

1. An **eval config** (`eval.yaml`) that ties everything together
2. An **MCP server config** pointing at the server you want to test
3. One or more **tasks** that define what the agent should do

Here is a minimal example:

**eval.yaml**:
```yaml
kind: Eval
metadata:
  name: "my-first-eval"
config:
  agent:
    type: "builtin.claude-code"
  mcpConfigFile: mcp-config.yaml
  taskSets:
    - path: tasks/create-pod.yaml
```

**mcp-config.yaml**:
```yaml
mcpServers:
  kubernetes:
    type: http
    url: http://localhost:8080/mcp
    enableAllTools: true
```

**tasks/create-pod.yaml**:
```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: "create-nginx-pod"
  difficulty: easy

spec:
  setup:
    - script:
        file: setup.sh

  verify:
    - script:
        file: verify.sh

  cleanup:
    - script:
        file: cleanup.sh

  prompt:
    inline: Create a nginx pod named web-server in the test-namespace
```

Run the evaluation:

```bash
mcpchecker check eval.yaml
```

The tool displays progress in real-time, saves results to `mcpchecker-<name>-out.json`, and prints a pass/fail summary.

For hands-on tutorials, see [Quickstarts](https://github.com/mcpchecker/quickstarts).

## Next Steps

- [Configure agents](how-to/configure-agents.md) -- set up Claude Code, LLM agents, or custom agents
- [Write tasks](how-to/write-tasks.md) -- define what agents should do and how to verify it
- [Use assertions](how-to/use-assertions.md) -- validate agent behavior beyond pass/fail
- [CLI reference](reference/cli/mcpchecker.md) -- all available commands and flags
