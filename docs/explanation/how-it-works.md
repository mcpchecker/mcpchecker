# How It Works

mcpchecker validates MCP servers by running real AI agents against them and checking the results.

## The MCP Proxy

The core mechanism is an MCP proxy that sits between the AI agent and your MCP server:

```
AI Agent --> MCP Proxy (recording) --> Your MCP Server
```

The proxy records everything:
- Which tools were called
- What arguments were passed
- When calls happened
- What responses came back

After the agent finishes its task, mcpchecker runs your verification steps (scripts or LLM judge) and checks assertions against the recorded behavior.

## Evaluation Flow

For each task, mcpchecker follows this sequence:

1. **Setup** -- Run setup scripts to prepare the environment (e.g., create a test namespace)
2. **Agent execution** -- Give the agent a prompt and let it work, with the MCP proxy recording all tool use
3. **Verification** -- Run verification scripts or LLM judge to check whether the task succeeded
4. **Assertion checking** -- Validate the recorded behavior against your assertions (e.g., did the agent call the right tools?)
5. **Cleanup** -- Run cleanup scripts to tear down resources

## What This Tells You

If agents successfully complete tasks using your MCP server, it means your tools are well-designed: they are discoverable, the descriptions are clear enough for an LLM to understand, the schemas work correctly, and the implementation handles real use cases.

If tasks fail, the recorded call history and assertion results help you pinpoint what went wrong -- whether the agent could not find the right tool, misunderstood the parameters, or the tool itself returned unexpected results.
