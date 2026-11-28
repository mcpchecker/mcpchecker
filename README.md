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

## Tool Coverage Output

After all tasks finish, `check` compares recorded calls across all task runs with
a snapshot of each enabled server's advertised tools. The saved JSON includes
the following `toolCoverage` structure alongside `summary` and `results`:

```json
{
  "toolCoverage": [
    {
      "serverName": "kubernetes",
      "discoveryOk": true,
      "tools": [
        {"name": "nodes_top", "called": true, "successCount": 2, "failureCount": 1},
        {"name": "api_resources_list", "called": false, "successCount": 0, "failureCount": 0}
      ]
    }
  ]
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `toolCoverage` | array of server objects | Omitted when no MCP servers are configured. |
| `serverName` | string | Server name from the MCP configuration. |
| `discoveryOk` | boolean | Whether the server's tool list was successfully discovered. |
| `discoveryError` | string, optional | Discovery failure details, including a timeout. Omitted on success. |
| `tools` | array of tool objects | Advertised tools, including tools excluded by configuration and tools with no calls. Empty if discovery fails or the server has no tools; use `discoveryOk` to distinguish these cases. |
| `tools[].name` | string | Tool name. |
| `tools[].called` | boolean | Whether at least one recorded call attempted this tool, even if it failed. |
| `tools[].successCount` | integer | Number of successful calls. |
| `tools[].failureCount` | integer | Number of failed calls, including request errors and MCP responses marked `isError`. |

In the example, `nodes_top` was attempted three times: two calls succeeded and
one failed. Requesting metrics for a nonexistent node is one way a call can
fail. Failed attempts still count toward tool coverage, so `called` is `true`
even when every recorded call to a tool fails.

A tool that was never called has `called: false`, `successCount: 0`, and
`failureCount: 0`, as shown by `api_resources_list`. The failure count increases
only when a recorded call fails; leaving a tool unused does not count as a
failure. Adding `successCount` and `failureCount` gives the total number of
recorded attempts for that tool across all tasks and repeated runs.

Coverage discovery has a 30-second timeout per server. Coverage measures tool
attempts; it does not track which parameters were used. See the
[output format reference](docs/reference/output-format.md#tool-coverage) for details.

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
