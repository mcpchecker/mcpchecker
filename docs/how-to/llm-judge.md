# LLM Judge Verification

Instead of verifying task results with scripts, you can use an LLM judge to semantically evaluate the agent's response. This is useful when:

- The expected output format may vary but the meaning should be consistent
- You want to check if the response contains specific information
- The agent provides text responses rather than performing observable actions

In the v1alpha2 task format, LLM judge steps can be combined with other verification steps in the same task.

## Setup

Configure the LLM judge in your `eval.yaml`:

```yaml
config:
  llmJudge:
    env:
      baseUrlKey: JUDGE_BASE_URL      # Env var name for LLM API base URL
      apiKeyKey: JUDGE_API_KEY        # Env var name for LLM API key
      modelNameKey: JUDGE_MODEL_NAME  # Env var name for model name
```

Set the environment variables before running:

```bash
export JUDGE_BASE_URL="https://api.openai.com/v1"
export JUDGE_API_KEY="sk-..."
export JUDGE_MODEL_NAME="gpt-4o"
```

The LLM judge supports any OpenAI-compatible API. The implementation uses the OpenAI Go SDK with a configurable base URL, so any endpoint that follows the OpenAI API format will work.

## Evaluation Modes

### Contains

Checks whether the agent's response semantically contains the expected information. Extra, non-contradictory information is acceptable. Format and phrasing differences are ignored.

Use this when you want to confirm the response includes specific facts without requiring an exact match.

### Exact

Checks whether the agent's response is semantically equivalent to a reference answer. Simple rephrasing is acceptable (e.g., "Paris is the capital" vs "The capital is Paris"), but adding or omitting information will fail.

Use this when you need precise semantic equivalence.

## Usage in Tasks (v1alpha2)

In the v1alpha2 format, `llmJudge` is a step type in the verify phase. You can use it alongside other verification steps:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: "check-image-version"
  difficulty: easy

spec:
  verify:
    # Script-based check
    - script:
        file: ./verify-pod-running.sh

    # LLM judge check (in the same task)
    - llmJudge:
        contains: "mysql:8.0.36"

  prompt:
    inline: What container image is the web-server pod running?
```

Using `exact` mode:

```yaml
spec:
  verify:
    - llmJudge:
        exact: "The pod web-server is running in namespace test-ns"
```

## Usage in Tasks (v1alpha1 / Legacy)

In the legacy format, LLM judge verification replaces script-based verification -- you cannot use both in the same task:

```yaml
kind: Task
metadata:
  name: "check-image-version"
  difficulty: easy
steps:
  verify:
    contains: "mysql:8.0.36"
  prompt:
    inline: What container image is the web-server pod running?
```

## Implementation Details

Both modes use the same LLM-based evaluation approach. The difference is in the system prompt given to the judge. See [`pkg/llmjudge/prompts.go`](../../pkg/llmjudge/prompts.go) for details.
