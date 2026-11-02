# Use Assertions

Assertions let you validate agent behavior beyond simple pass/fail. You can check which tools were called, enforce call limits, verify resource access, and more.

Assertions can be defined in your task YAML or in your eval config under a task set. Eval task-set assertions apply to each matching task:

```yaml
taskSets:
  - path: tasks/create-pod.yaml
    assertions:
      toolsUsed:
        - server: kubernetes
          toolPattern: "pods_.*"
      minToolCalls: 1
      maxToolCalls: 10
```

## Task-level Assertions and Composition

Define assertions on an individual task under `spec.assertions`:

```yaml
spec:
  assertions:
    toolsUsed:
      - server: kubernetes
        tool: pods_create
```

By default, a missing targeted server or tool is skipped. Set `requirePresent: true` in an assertion set to make a missing target fail instead:

```yaml
spec:
  assertions:
    requirePresent: true
    toolsUsed:
      - server: kubernetes
        tool: pods_create
```

Task-level assertions are combined with assertions from every matching eval task set. The task-level set is added once even if the task matches multiple task sets. Each set is evaluated independently, and **all assertion sets must pass** for assertions to pass. For example, this eval-wide assertion applies to tasks matched by the glob:

```yaml
config:
  taskSets:
    - glob: tasks/*.yaml
      assertions:
        maxToolCalls: 20
```

Presence behavior is the same for task-level and eval-level assertions. “Present” means the target is reported by the live server and, for tools, remains accessible after configured tool filtering. A discovery error is an evaluation error; it is not treated as a missing capability.

By default, absent targets in `toolsUsed` and `toolsNotUsed` are skipped. A `callOrder` assertion is skipped if any of its targets is absent. `requireAny` evaluates the candidates that are present; if none are present, the assertion is skipped. With `requirePresent: true`, these absences are failures. Resource and prompt assertions currently check only whether their server is present; they do not check for a specific resource or prompt. An absent server skips those assertions by default and fails them with `requirePresent: true`.

`minToolCalls`, `maxToolCalls`, `noDuplicateCalls`, and skill assertions are not capability-gated. They are evaluated regardless of whether a particular server or tool is present.

### Tool-removal experiment

Capability-aware skipping is useful when comparing an MCP server before and after removing a tool:

1. **Baseline:** the tool is present, and the agent uses it; the tool assertion is evaluated and passes.
2. **Tool removed:** the targeted tool is no longer available, so its assertion is skipped by default.
3. **Task outcome:** task verification still determines whether the agent succeeded by another route. The skipped assertion does not establish task success.

## Tool Usage

### Required Tools

Check that the agent called specific tools:

```yaml
assertions:
  toolsUsed:
    - server: kubernetes
      tool: pods_create              # Exact tool name
    - server: kubernetes
      toolPattern: "pods_.*"         # Regex pattern
```

### Required (Any Of)

Check that the agent called at least one tool from a set:

```yaml
assertions:
  requireAny:
    - server: kubernetes
      tool: pods_create
```

### Forbidden Tools

Check that the agent did not call certain tools:

```yaml
assertions:
  toolsNotUsed:
    - server: kubernetes
      tool: namespaces_delete
```

## Call Limits

Set bounds on how many tool calls the agent made:

```yaml
assertions:
  minToolCalls: 1
  maxToolCalls: 10
```

## Resource Access

### Required Resources

Check that specific resources were read:

```yaml
assertions:
  resourcesRead:
    - server: filesystem
      uriPattern: "/data/.*\\.json$"
```

### Forbidden Resources

Check that sensitive resources were not accessed:

```yaml
assertions:
  resourcesNotRead:
    - server: filesystem
      uri: /etc/secrets/password
```

## Prompt Usage

Check that the agent used specific prompts:

```yaml
assertions:
  promptsUsed:
    - server: templates
      prompt: deployment-template
```

## Call Order

Verify tools were called in a specific sequence. Other calls can happen between the listed ones:

```yaml
assertions:
  callOrder:
    - type: tool
      server: kubernetes
      name: namespaces_create
    - type: tool
      server: kubernetes
      name: pods_create
```

## No Duplicate Calls

Ensure the agent did not make redundant calls:

```yaml
assertions:
  noDuplicateCalls: true
```

## Full Example

Here is an eval config that uses several assertion types together:

```yaml
kind: Eval
metadata:
  name: "kubernetes-test"
config:
  agent:
    type: "builtin.claude-code"
  mcpConfigFile: mcp-config.yaml
  taskSets:
    - path: tasks/create-pod.yaml
      assertions:
        toolsUsed:
          - server: kubernetes
            tool: pods_create
        toolsNotUsed:
          - server: kubernetes
            tool: namespaces_delete
        callOrder:
          - type: tool
            server: kubernetes
            name: namespaces_create
          - type: tool
            server: kubernetes
            name: pods_create
        minToolCalls: 1
        maxToolCalls: 10
```

## MCP Apps

Use `assertions.apps` with any agent using the recording proxy to check that
invoked tools provide retrievable, nonempty HTML UI resources with the MCP Apps
MIME type. Assertions activate collection automatically. Separately, opt into
Apps protocol handling for an agent with `config.agent.apps.host: true` to
advertise UI support, hide app-only tools, and filter UI metadata from results
and their content blocks.
No browser is required. This does not render the app or validate its elements or
displayed data. See [Evaluate MCP Apps resources](mcp-apps.md)
for configuration and a namespace example. Missing servers or model-visible tools
are skipped unless `requirePresent: true`. A present tool that is not called fails
with `tool_not_invoked`; UI resource validation failures use separate failure codes.
