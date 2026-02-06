# Task Format

This document describes the task format used by mcpchecker to define evaluation tasks.

## Overview

Tasks define what an agent should do and how to verify it succeeded. Each task specifies:

- A prompt to give the agent
- Optional setup steps to run before the agent
- Verification steps to check the agent's work
- Optional cleanup steps to run after verification

mcpchecker supports two API versions:

- `mcpchecker/v1alpha2` - Declarative step-based format (recommended for new tasks)
- `mcpchecker/v1alpha1` - Legacy script-based format (still supported)

## Task Schema

A v1alpha2 task has the following structure:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: string        # Required. Unique task identifier.
  difficulty: string  # Optional. One of: easy, medium, hard.

spec:
  requires:           # Optional. Extension requirements.
    - extension: string

  setup:              # Optional. Steps to run before the agent.
    - stepType: { ... }

  verify:             # Required. Steps to check task success.
    - stepType: { ... }

  cleanup:            # Optional. Steps to run after verification.
    - stepType: { ... }

  prompt:             # Required. What to tell the agent.
    inline: string    # Inline prompt text.
    # or
    file: string      # Path to prompt file.
```

### Step Format

Each step is a single-key map where the key is the step type and the value is the step configuration:

```yaml
- http:
    url: https://example.com
    expect:
      status: 200

- script:
    file: ./verify.sh
```

## Built-in Step Types

mcpchecker provides three built-in step types.

### http

Makes an HTTP request and optionally validates the response.

```yaml
- http:
    url: string               # Required. URL to request.
    method: string            # Optional. Default: GET.
    headers:                  # Optional. Request headers.
      Header-Name: value
    body:                     # Optional. Request body (one of):
      raw: string             #   Raw string body.
      json: { ... }           #   JSON object body.
    timeout: string           # Optional. Default: 5m. Duration format (e.g., 30s, 2m).
    expect:                   # Optional. Response validation.
      status: number          #   Expected status code.
      body:                   #   Body validation.
        match: regex          #     Regex pattern on raw body.
        fields:               #     JSON field assertions.
          - path: string      #       Dot notation path (e.g., data.user.name, items[0].id).
            equals: any       #       Expected value.
            type: string      #       Expected type: string, number, array, object, bool, null.
            match: regex      #       Regex for string values.
            exists: boolean   #       Field presence check.
```

**Example:**

```yaml
- http:
    url: http://localhost:8080/api/users
    method: GET
    headers:
      Authorization: Bearer token123
    expect:
      status: 200
      body:
        fields:
          - path: data.users
            type: array
          - path: data.users[0].email
            match: ".*@example\\.com"
```

### script

Runs a script file or inline script content.

```yaml
- script:
    file: string            # Path to script file (relative to task directory).
    # or
    inline: string          # Inline script content.

    timeout: string         # Optional. Default: 5m. Duration format.
    continueOnError: bool   # Optional. Default: false. If true, step failure does not stop execution.
```

Scripts with a shebang (`#!/usr/bin/env python3`) are executed directly. Scripts without a shebang are executed using the shell specified by `$SHELL` or `/usr/bin/bash`.

**Example with file:**

```yaml
- script:
    file: ./verify.sh
    timeout: 2m
```

**Example with inline script:**

```yaml
- script:
    inline: |
      #!/usr/bin/env python3
      import subprocess
      result = subprocess.run(["kubectl", "get", "pod", "nginx"], capture_output=True)
      exit(0 if result.returncode == 0 else 1)
```

### llmJudge

Uses an LLM to evaluate the agent's response. Only valid in the verify phase. Requires an LLM judge to be configured in the eval.yaml. Supports both OpenAI and Claude (Anthropic) models.

```yaml
- llmJudge:
    contains: string   # Semantic containment check.
    # or
    exact: string      # Semantic equivalence check.
```

One of `contains` or `exact` must be specified, but not both.

- `contains` - Passes if the agent's response semantically contains the expected information.
- `exact` - Passes if the agent's response is semantically equivalent to the expected answer.

**Example:**

```yaml
- llmJudge:
    contains: "The pod is running in the default namespace"
```

**Configuring the LLM Judge:**

The judge is configured in your `eval.yaml` file. You can use either OpenAI or Claude as your judge.

**OpenAI Judge Configuration:**

```yaml
llmJudge:
  env:
    typeKey: JUDGE_TYPE           # Set to "openai"
    baseUrlKey: JUDGE_BASE_URL    # e.g., https://api.openai.com/v1
    apiKeyKey: JUDGE_API_KEY      # Your OpenAI API key
    modelNameKey: JUDGE_MODEL_NAME # e.g., gpt-4o
```

**Claude Judge Configuration:**

The Claude judge uses the Claude Code CLI (`claude` binary). Install it from https://github.com/anthropics/claude-code

```yaml
llmJudge:
  env:
    typeKey: JUDGE_TYPE  # Set to "claude"
```

Environment variables:
```bash
export JUDGE_TYPE="claude"
```

**Note**: API keys and model names are not required for Claude judge since it uses the local CLI binary.

**Note:** If `typeKey` is not specified or the environment variable is not set, the judge defaults to OpenAI for backward compatibility.

## Using Extensions

Extensions provide domain-specific operations (e.g., Kubernetes resource management). To use an extension:

1. Declare the extension requirement in the task
2. Configure the extension in eval.yaml
3. Use extension operations as steps

### Declaring Extension Requirements

In your task file, declare which extensions the task needs:

```yaml
spec:
  requires:
    - extension: kubernetes
```

If you want to rename the extension, for example to avoid naming conflicts or to make it easier to type, you can set an alias:

```yaml
spec:
  requires:
    - extension: kubernetes
      as: k8s
```

Throughout the rest of the task, you can now refer to the kubernetes extension with `k8s` instead of `kubernetes`.

### Configuring Extensions in eval.yaml

Extensions are configured in the eval.yaml under `config.extensions`:

```yaml
kind: Eval
metadata:
  name: my-eval
config:
  extensions:
    kubernetes:                          # Extension alias (matches spec.requires)
      package: https://github.com/mcpchecker/kubernetes-extension@v0.0.1
      config:                            # Optional. Extension-specific configuration.
        kubeconfig: ~/.kube/config
      env:                               # Optional. Environment variables for the extension.
        KUBECONFIG: /path/to/config
```

The `package` field can be:
- A GitHub package reference (e.g., `https://github.com/org/repo@v1.0.0`)
- A relative or absolute path to a local binary (for development)

### Using Extension Operations

Extension operations use the syntax `extension.operation`:

```yaml
setup:
  - kubernetes.create: # or k8s.create if you used an alias
      apiVersion: v1
      kind: Namespace
      metadata:
        name: test-namespace

verify:
  - kubernetes.wait:
      apiVersion: v1
      kind: Pod
      metadata:
        name: my-pod
        namespace: test-namespace
      condition: Ready
      timeout: 120s

cleanup:
  - kubernetes.delete:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: test-namespace
      ignoreNotFound: true
```

The arguments passed to each operation depend on the extension. Extensions define their operations and parameter schemas in their manifest. See the extension's documentation for available operations.

## Complete Example

Here is a complete v1alpha2 task that creates and verifies a Kubernetes pod:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: create-nginx-pod
  difficulty: easy

spec:
  requires:
    - extension: kubernetes

  setup:
    # Clean up any existing namespace first
    - kubernetes.delete:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test
        ignoreNotFound: true

    # Create a fresh namespace
    - kubernetes.create:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test

  verify:
    # Wait for the pod to be ready
    - kubernetes.wait:
        apiVersion: v1
        kind: Pod
        metadata:
          name: web-server
          namespace: create-pod-test
        condition: Ready
        timeout: 120s

  cleanup:
    # Delete the namespace to clean up all resources
    - kubernetes.delete:
        apiVersion: v1
        kind: Namespace
        metadata:
          name: create-pod-test
        ignoreNotFound: true

  prompt:
    inline: Please create a nginx pod named web-server in the create-pod-test namespace
```

## Legacy Format (v1alpha1)

The legacy format uses script files for setup, verify, and cleanup:

```yaml
kind: Task
metadata:
  name: my-task
  difficulty: easy
steps:
  setup:
    file: setup.sh
  verify:
    file: verify.sh
  cleanup:
    file: cleanup.sh
  prompt:
    inline: "Do something"
```

This format is still supported. Tasks without an `apiVersion` field or with `apiVersion: mcpchecker/v1alpha1` use this format.

The v1alpha1 format also supports LLM judge verification:

```yaml
steps:
  verify:
    contains: "expected semantic content"
    # or
    exact: "expected exact answer"
```

## Migrating from v1alpha1 to v1alpha2

To convert a legacy task to the new format, replace script file references with `script` steps.

**Before (v1alpha1):**

```yaml
kind: Task
metadata:
  name: my-task
  difficulty: medium
steps:
  setup:
    file: setup.sh
  verify:
    file: verify.sh
  cleanup:
    file: cleanup.sh
  prompt:
    inline: "Do something"
```

**After (v1alpha2):**

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: my-task
  difficulty: medium
spec:
  setup:
    - script:
        file: setup.sh

  verify:
    - script:
        file: verify.sh

  cleanup:
    - script:
        file: cleanup.sh

  prompt:
    inline: "Do something"
```

Key changes:

1. Add `apiVersion: mcpchecker/v1alpha2`
2. Move task definition under `spec`
3. Replace `steps.setup.file: setup.sh` with a `script` step in `spec.setup`
4. Each phase (`setup`, `verify`, `cleanup`) is now an array of steps

You can also inline scripts directly:

```yaml
spec:
  verify:
    - script:
        inline: |
          #!/bin/bash
          kubectl get pod my-pod -o jsonpath='{.status.phase}' | grep -q Running
```

Once migrated, you can incrementally replace script steps with built-in steps or extension operations to reduce bash complexity.

## Extension Protocol

Extensions communicate with mcpchecker using JSON-RPC 2.0 over stdio. For details on implementing an extension, see [Extension Protocol Specification](specs/extension-protocol.md).
