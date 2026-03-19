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
