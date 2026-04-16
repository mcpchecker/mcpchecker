# Output Format

mcpchecker saves evaluation results to `mcpchecker-<eval-name>-out.json`. This file contains the resolved configuration summary and the full record of each task run, including pass/fail status, assertion results, and call history.

## Top-Level Structure

The output file is a JSON object with two top-level fields:

```json
{
  "summary": { ... },
  "results": [ ... ]
}
```

### Summary

The `summary` object captures the resolved configuration used for the evaluation run. This makes the output self-documenting — you can always tell which agent, model, judge, and MCP servers were used.

```json
{
  "summary": {
    "agent": {
      "type": "builtin.llm-agent",
      "name": "my-agent",
      "model": "openai:gpt-5",
      "path": "agents/my-agent.yaml",
      "command": "node agent.js"
    },
    "judge": {
      "type": "builtin.llm-agent",
      "name": "my-judge",
      "model": "claude-sonnet-4",
      "path": "agents/my-judge.yaml",
      "command": "node judge.js"
    },
    "mcpServers": [
      {
        "name": "kubernetes",
        "type": "http",
        "url": "http://localhost:8080/mcp"
      }
    ],
    "evals": {
      "names": ["create-pod", "list-pods", "delete-pod"],
      "taskSets": [
        {
          "glob": "../tasks/kubernetes/*.yaml",
          "labelSelector": { "suite": "kubernetes" }
        }
      ]
    },
    "timeout": {
      "defaultTask": "5m"
    },
    "parallelWorkers": 1,
    "runs": 1
  },
  "results": [ ... ]
}
```

### Results

The `results` array contains one entry per task run. Each entry has the following structure:

```json
{
  "taskName": "create-nginx-pod",
  "taskPath": "tasks/kubernetes/create-pod.yaml",
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

> **Legacy format:** Older output files (pre-summary) used a bare JSON array at the top level. All CLI commands (`view`, `summary`, `diff`, `verify`) auto-detect and support both formats. Support for the legacy format is deprecated and will be removed in a future release — re-run evaluations to generate output in the current format.

## Interpreting Results

**Pass** means the agent successfully completed the task using your MCP server. Your tools are discoverable, descriptions are clear, schemas work, and the implementation is correct.

**Fail** means something needs improvement. Common causes:
- Tool descriptions are unclear to the agent
- Tool schemas are too complex
- Missing functionality the agent expected
- Implementation bugs in the MCP server

## Viewing Results

Use the CLI to inspect results:

```bash
# Pretty-print results
mcpchecker result view mcpchecker-my-eval-out.json

# Show a compact summary
mcpchecker result summary mcpchecker-my-eval-out.json

# Compare two runs
mcpchecker result diff run1-out.json run2-out.json

# Verify results meet thresholds
mcpchecker result verify mcpchecker-my-eval-out.json
```

See the [CLI reference](cli/mcpchecker.md) for full details on each command.
