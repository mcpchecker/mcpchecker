# Output Format

mcpchecker saves evaluation results to `mcpchecker-<eval-name>-out.json`. This file contains the full record of each task run, including pass/fail status, assertion results, and call history.

## Result Structure

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
mcpchecker view mcpchecker-my-eval-out.json

# Show a compact summary
mcpchecker summary mcpchecker-my-eval-out.json

# Compare two runs
mcpchecker diff run1-out.json run2-out.json

# Verify results meet thresholds
mcpchecker verify mcpchecker-my-eval-out.json
```

See the [CLI reference](cli/mcpchecker.md) for full details on each command.
