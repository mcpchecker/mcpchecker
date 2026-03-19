# mcpchecker

Test your MCP servers by having AI agents complete real tasks.

## Why mcpchecker?

You've built an MCP server with tools. It works. But can an AI agent actually discover and use your tools correctly? Are your descriptions clear enough? Does your server handle edge cases?

mcpchecker answers these questions automatically. It runs real AI agents against your MCP server, records every tool call, and verifies that tasks complete successfully. Think of it as integration testing for AI tool use.

## Install

```bash
brew tap mcpchecker/mcpchecker
brew install mcpchecker
```

For other platforms (Linux, manual download), see [Getting Started](docs/getting-started.md).

## Quick Start

```bash
mcpchecker check eval.yaml
```

This runs an evaluation that:
1. Starts your MCP server and sets up an MCP proxy to record tool calls
2. Gives an AI agent a task prompt
3. Verifies the task succeeded (via scripts or LLM judge)
4. Checks assertions against the recorded behavior

Results are saved to `mcpchecker-<name>-out.json` with a pass/fail summary printed to the terminal.

For hands-on tutorials, see [Quickstarts](https://github.com/mcpchecker/quickstarts).

## How It Works

mcpchecker places a recording proxy between the agent and your MCP server:

```text
AI Agent --> MCP Proxy (recording) --> Your MCP Server
```

If agents can discover and use your tools to complete tasks, your server is well-designed. If they can't, the recorded call history helps you figure out why.

Read more in [How It Works](docs/explanation/how-it-works.md).

## Documentation

**Getting started:**
- [Installation and first run](docs/getting-started.md)

**How-to guides:**
- [Configure agents](docs/how-to/configure-agents.md) -- Claude Code, LLM agents, custom agents, ACP mode
- [Write tasks](docs/how-to/write-tasks.md) -- task structure, labels, filtering, extensions
- [Use assertions](docs/how-to/use-assertions.md) -- validate tool usage, call order, resource access
- [LLM judge verification](docs/how-to/llm-judge.md) -- semantic evaluation of agent responses
- [Parallel execution and multi-run](docs/how-to/parallel-and-multi-run.md) -- speed up evals and test consistency

**Reference:**
- [CLI commands](docs/reference/cli/mcpchecker.md)
- [Task format](docs/reference/task-format.md)
- [Output format](docs/reference/output-format.md)

**Understanding:**
- [How it works](docs/explanation/how-it-works.md) -- architecture and evaluation flow

## Building from Source

```bash
go build -o mcpchecker ./cmd/mcpchecker
```

## License

See [LICENSE](LICENSE).
