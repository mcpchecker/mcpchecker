# Write Tasks

Tasks define what an agent should do and how to verify the result. Each task includes a prompt for the agent, optional setup and cleanup steps, and verification steps.

For the full task YAML schema (including extensions and the legacy v1alpha1 format), see the [Task Format Reference](../reference/task-format.md).

## Basic Structure

A task file uses the v1alpha2 format:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: "create-nginx-pod"
  difficulty: easy

spec:
  setup:
    - script:
        inline: |
          kubectl create namespace test-ns

  verify:
    - script:
        file: ./verify.sh

  cleanup:
    - script:
        inline: |
          kubectl delete namespace test-ns

  prompt:
    inline: Create a nginx pod named web-server in the test-ns namespace
```

- **setup** runs before the agent (e.g., create a test namespace)
- **prompt** is what the agent receives
- **verify** checks whether the agent succeeded
- **cleanup** tears down resources after verification

The phases `setup`, `verify`, and `cleanup` are arrays of steps, so you can chain multiple operations. The `prompt` is a single step object (not an array).

## Step Types

### Script

Runs a script file or inline script. Exit code 0 means success.

```yaml
# From a file
- script:
    file: ./verify.sh
    timeout: 2m

# Inline
- script:
    inline: |
      #!/usr/bin/env bash
      kubectl wait --for=condition=Ready pod/web-server -n test-ns --timeout=120s
```

### HTTP

Makes an HTTP request and validates the response:

```yaml
- http:
    url: http://localhost:8080/api/users
    method: GET
    expect:
      status: 200
      body:
        fields:
          - path: data.users
            type: array
```

### LLM Judge

Semantically evaluates the agent's response (only valid in the verify phase). See [LLM Judge Verification](llm-judge.md) for setup and details.

```yaml
- llmJudge:
    contains: "The pod is running in the default namespace"
```

## Using Extensions

Extensions provide domain-specific operations. For example, the Kubernetes extension gives you declarative steps for creating, waiting on, and deleting resources:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: "create-nginx-pod"
  difficulty: easy

spec:
  requires:
    - extension: kubernetes

  setup:
    - kubernetes.delete:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test
        ignoreNotFound: true
    - kubernetes.create:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test

  verify:
    - kubernetes.wait:
        apiVersion: v1
        kind: Pod
        metadata:
          name: web-server
          namespace: create-pod-test
        condition: Ready
        timeout: 120s

  cleanup:
    - kubernetes.delete:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test
        ignoreNotFound: true

  prompt:
    inline: Please create a nginx pod named web-server in the create-pod-test namespace
```

## Organizing Tasks with Labels

Add labels to tasks for categorization and filtering:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: "create-pod"
  difficulty: easy
  labels:
    suite: kubernetes
    category: basic
    requires: cluster
```

## Filtering with Label Selectors

Use `labelSelector` in your eval config to run specific subsets of tasks:

```yaml
taskSets:
  # Run only kubernetes tasks
  - glob: tasks/**/*.yaml
    labelSelector:
      suite: kubernetes

  # Combine multiple labels (AND logic -- all must match)
  - glob: tasks/**/*.yaml
    labelSelector:
      suite: kiali
      requires: istio
```

You can also define multiple task sets with different filters and assertions:

```yaml
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

**How label selectors work:**
- All labels in the selector must match (AND logic)
- If `labelSelector` is omitted or empty, all tasks matched by the glob/path are included
- Tasks without labels will not match any non-empty label selector
- Both the glob/path pattern and label selector must match for a task to be included

**Tips:**
- Use consistent label keys across your task suite (`suite`, `category`, `requires`, etc.)
- Combine directory structure with labels for flexible organization
- Use globs for path-based filtering, labels for semantic filtering

## Task Timeouts

You can set timeout limits to prevent tasks from running indefinitely. This is useful when agents get stuck in loops or when tasks interact with slow external services.

### Per-task timeout

Add `limits` to the task spec:

```yaml
spec:
  limits:
    timeout: "15m"       # Max time for setup + agent + verify
    cleanupTimeout: "2m" # Max time for cleanup (runs even after timeout)

  setup:
    # ...
```

### Eval-level defaults

Set defaults for all tasks in your eval config with `defaultTaskLimits`:

```yaml
config:
  defaultTaskLimits:
    timeout: "30m"
    cleanupTimeout: "5m"
  agent:
    type: "builtin.claude-code"
  taskSets:
    - glob: tasks/**/*.yaml
```

Tasks with their own `spec.limits` override these defaults.

### CLI overrides

Override timeouts from the command line:

```bash
# Set a default for tasks that don't specify their own
mcpchecker check eval.yaml --default-task-timeout 20m

# Hard override ALL tasks (even those with spec.limits)
mcpchecker check eval.yaml --task-timeout 10m

# Same pattern for cleanup
mcpchecker check eval.yaml --default-cleanup-timeout 5m
mcpchekcer check eval.yaml --cleanup-timeout 5m
```

For the full precedence rules, see [Task Timeouts](../reference/task-format.md#task-timeouts) in the reference.

## Eval Config with Assertions

A complete eval config ties together the agent, MCP server, and tasks:

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
            toolPattern: "pods_.*"
        minToolCalls: 1
        maxToolCalls: 10
```

For more on assertions, see [Use Assertions](use-assertions.md).

For LLM-based verification, see [LLM Judge Verification](llm-judge.md).

## Using External Task Sources

You can pull tasks from a GitHub repository instead of writing them locally. This is useful for consuming shared community task suites.

### Declare a source

Add a `sources` block to your eval config:

```yaml
kind: Eval
metadata:
  name: "k8s-eval"
config:
  agent:
    type: "builtin.claude-code"
  mcpConfigFile: mcp-config.yaml
  sources:
    k8s-tasks:
      repo: github.com/example/k8s-tasks
      ref: main          # branch, tag, or full commit SHA
      path: tasks/       # optional subdirectory within the repo
```

### Reference the source in a taskSet

Use the `source` field instead of `glob` or `path`:

```yaml
  taskSets:
    - source: k8s-tasks
      glob: "**/*.yaml"
      labelSelector:
        suite: kubernetes
```

### Fetch and lock

Before running `check`, fetch the source and write a lockfile:

```bash
mcpchecker install
```

This resolves the ref to a commit SHA, downloads the repository, and writes `mcpchecker.lock`. Commit the lockfile alongside `eval.yaml` to pin the source version for reproducible evaluations.

To refresh to the latest commit:

```bash
mcpchecker install --update
```

### Server name mapping

External tasks may reference MCP server names that differ from the names in your local MCP config. During `mcpchecker install`, you are prompted to map each unknown name to a local server:

```
Map "kubernetes" → [k8s-prod]: k8s-prod
```

The mapping is written to `eval.yaml` under the source's `serverMapping` key and applied automatically at eval time. You can also set it manually:

```yaml
  sources:
    k8s-tasks:
      repo: github.com/example/k8s-tasks
      ref: main
      serverMapping:
        kubernetes: k8s-prod   # tasks asking for "kubernetes" use "k8s-prod"
```

Or apply a mapping at the `taskSet` level to override for a specific set:

```yaml
  taskSets:
    - source: k8s-tasks
      glob: "**/*.yaml"
      serverMapping:
        kubernetes: k8s-staging
```
