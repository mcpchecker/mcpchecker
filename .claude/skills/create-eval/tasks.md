# Task Configuration Reference

Tasks define individual test scenarios that evaluate an agent's ability to complete specific objectives using MCP tools.

## Task YAML Structure

```yaml
kind: Task
metadata:
  name: "task-name"
  difficulty: "easy|medium|hard"
steps:
  setup:
    inline: "..." # OR file: "path/to/setup.sh"
  cleanup:
    inline: "..." # OR file: "path/to/cleanup.sh"
  verify:
    inline: "..." # OR file: "path/to/verify.sh"
  prompt:
    inline: "..." # OR file: "path/to/prompt.txt"
```

## Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `kind` | string | Yes | Must be `"Task"` |
| `metadata` | object | Yes | Task metadata (see below) |
| `steps` | object | Yes | Task execution steps (see below) |

## metadata Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Unique identifier for the task |
| `difficulty` | string | Yes | One of: `"easy"`, `"medium"`, `"hard"` |
| `labels` | map[string]string | No | Key-value labels for categorizing and filtering tasks |
| `parallel` | bool | No | If true, task can run in parallel with other parallel tasks (default: false) |
| `runs` | int | No | Number of times to run this task for consistency testing (default: 1) |

## steps Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `setup` | object | No | Script to run before agent executes |
| `cleanup` | object | No | Script to run after task completes |
| `verify` | object | Yes | Script to check if task succeeded (exit 0 = pass) |
| `prompt` | object | Yes | The task description/instructions for the agent |

## Step Object (setup, cleanup, verify, prompt)

Each step object has one of these fields (not both):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `inline` | string | One required | Inline script/content |
| `file` | string | One required | Path to script/content file |

**Important**: Exactly one of `inline` or `file` must be set for each step.

## Examples

### Minimal Task (inline)

```yaml
kind: Task
metadata:
  name: "simple-task"
  difficulty: easy
steps:
  prompt:
    inline: "Create a file named test.txt with the content 'Hello World'"
  verify:
    inline: |
      #!/usr/bin/env bash
      [ -f test.txt ] && grep -q "Hello World" test.txt
```

### Full Task with Setup and Cleanup

```yaml
kind: Task
metadata:
  name: "kubernetes-pod-creation"
  difficulty: medium
steps:
  setup:
    inline: |
      #!/usr/bin/env bash
      kubectl create namespace test-ns
  prompt:
    inline: "Create a nginx pod named web-server in the test-ns namespace"
  verify:
    inline: |
      #!/usr/bin/env bash
      kubectl wait --for=condition=Ready pod/web-server -n test-ns --timeout=120s
  cleanup:
    inline: |
      #!/usr/bin/env bash
      kubectl delete namespace test-ns
```

### Task Using Files

```yaml
kind: Task
metadata:
  name: "file-based-task"
  difficulty: medium
steps:
  setup:
    file: ./scripts/setup.sh
  prompt:
    file: ./prompts/task-description.txt
  verify:
    file: ./scripts/verify.sh
  cleanup:
    file: ./scripts/cleanup.sh
```

## Best Practices

1. **Use setup for environment preparation**: Create necessary resources, namespaces, etc.
2. **Keep prompts clear and specific**: The agent should understand exactly what to do
3. **Make verify scripts robust**: Check actual state, not just command success (exit 0 = pass, non-zero = fail)
4. **Always cleanup**: Remove test resources to avoid conflicts with subsequent runs
5. **Choose appropriate difficulty**:
   - `easy`: Simple, single-step tasks
   - `medium`: Multi-step tasks requiring some planning
   - `hard`: Complex tasks requiring multiple tools and careful orchestration
