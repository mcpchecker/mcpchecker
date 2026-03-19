## mcpchecker summary

Show a compact summary of evaluation results

### Synopsis

Display a concise summary of evaluation results showing pass/fail status per task.

Supports multiple output formats:
  - text (default): Human-readable summary with colors
  - json: Machine-readable JSON output
  - --github-output: GitHub Actions format (key=value)

```
mcpchecker summary <results-file> [flags]
```

### Options

```
      --github-output   Output in GitHub Actions format (key=value)
  -h, --help            help for summary
  -o, --output string   Output format (text, json) (default "text")
      --task string     Filter results by task name
```

### SEE ALSO

* [mcpchecker](mcpchecker.md)	 - MCP evaluation framework

