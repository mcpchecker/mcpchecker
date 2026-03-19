# Use Assertions

Assertions let you validate agent behavior beyond simple pass/fail. You can check which tools were called, enforce call limits, verify resource access, and more.

Assertions are defined in your eval config under each task set:

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
