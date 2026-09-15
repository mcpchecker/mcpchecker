## mcpchecker install

Fetch dependencies and write lockfile

### Synopsis

Fetch all sources and extensions defined in an eval config, write or update the lockfile (`mcpchecker.lock`).

When installing a source for the first time, interactively maps MCP server names from the source tasks to server names in your MCP config.

```
mcpchecker install [eval-config-file] [flags]
```

### Options

```
  -h, --help     help for install
  -u, --update   Re-resolve all refs to current commits and update lockfile
```

### Subcommands

#### mcpchecker install source

Fetch a specific source by name.

```
mcpchecker install source <name> [eval-config-file] [flags]
```

```
  -h, --help     help for source
  -u, --update   Re-resolve ref to current commit
```

#### mcpchecker install extension

Record a specific extension by name.

```
mcpchecker install extension <name> [eval-config-file] [flags]
```

```
  -h, --help     help for extension
  -u, --update   Re-resolve to current version
```

### Lockfile

`mcpchecker install` writes `mcpchecker.lock` next to your eval config. The lockfile pins each source to a specific commit SHA and records a content hash for cache integrity:

```yaml
version: 1
sources:
  my-tasks:
    repo: github.com/example/my-tasks
    ref: main
    commit: abc123...
    hash: sha256:...
    fetchedAt: "2026-01-01T00:00:00Z"
extensions:
  my-ext:
    package: github.com/mcpchecker/kubernetes-extension@v0.0.2
    fetchedAt: "2026-01-01T00:00:00Z"
```

Commit `mcpchecker.lock` alongside `eval.yaml` to ensure reproducible evaluations. Run `mcpchecker install --update` to refresh pinned commits when you want to pull in upstream changes.

### Server Mapping

When a source's tasks reference MCP servers not found in your local MCP config, `install` prompts you to map each unknown name to a local server:

```
Detected MCP servers required by external tasks in source "k8s-tasks":
  - kubernetes (12 tasks)

Your MCP config defines:
  - k8s-prod
  - monitoring

Map "kubernetes" → [k8s-prod]: k8s-prod
```

The mapping is written to the `serverMapping` field of the source in `eval.yaml` and applied automatically at eval time.

### SEE ALSO

* [mcpchecker](mcpchecker.md) - MCP evaluation framework
* [mcpchecker check](mcpchecker_check.md) - Run an evaluation
