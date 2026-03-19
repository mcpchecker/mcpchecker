# Parallel Execution and Multi-Run

## Parallel Execution

When tasks are independent of each other, you can run them in parallel to speed up evaluations.

### Marking Tasks as Parallel

Add `parallel: true` to the task metadata:

```yaml
kind: Task
metadata:
  name: "create-nginx-pod"
  difficulty: easy
  parallel: true
steps:
  # ...
```

### Running in Parallel

Use the `-p` or `--parallel` flag to set the number of concurrent workers:

```bash
# Run with 4 parallel workers
mcpchecker check eval.yaml -p 4

# Sequential execution (default)
mcpchecker check eval.yaml
```

### Execution Order

When `--parallel > 1`:

1. Sequential tasks (without `parallel: true`) run first, one at a time, in order
2. Parallel tasks run together as a batch, limited by the worker count

This lets you run setup tasks sequentially before independent tasks run in parallel.

### When to Use Parallel

Mark a task as `parallel: true` when:
- It is independent and does not depend on state from other tasks
- It does not modify shared resources that other tasks rely on
- Its setup and cleanup scripts handle isolation properly

Keep tasks sequential (the default) when:
- They must run in a specific order
- They share state or resources
- One task depends on the output of another

## Multi-Run Execution

For consistency testing, you can run each task multiple times to measure how reliably an agent completes it.

### Task-Level Configuration

Set the `runs` field in task metadata:

```yaml
kind: Task
metadata:
  name: "fix-crashloop"
  difficulty: medium
  parallel: true
  runs: 4
```

### CLI Override

Use `-n` or `--runs` to override task-level runs for all tasks:

```bash
# Run each task 5 times
mcpchecker check eval.yaml -n 5

# Use task-level runs (default)
mcpchecker check eval.yaml
```

### Results

When running multiple times:
- Each run gets its own setup, agent, verify, cleanup cycle
- Progress shows `[run X/N]` for each run
- The summary shows per-task pass rate (e.g., "2/3 (66.7%)")
