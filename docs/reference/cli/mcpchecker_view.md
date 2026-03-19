## mcpchecker view

Pretty-print evaluation results from a JSON file

### Synopsis

Render the JSON output produced by "mcpchecker run" in a human-friendly format.

Examples:
  mcpchecker view mcpchecker-netedge-selector-mismatch-out.json
  mcpchecker view --task netedge-selector-mismatch --max-events 15 results.json

```
mcpchecker view <results-file> [flags]
```

### Options

```
  -h, --help                   help for view
      --max-events int         Maximum number of timeline entries (thought/command/tool/etc.) to display (0 = unlimited) (default 40)
      --max-line-length int    Maximum characters per line when formatting timeline output (default 100)
      --max-output-lines int   Maximum lines to display for command output in the timeline (default 6)
      --task string            Only show results for tasks whose name contains this value
      --timeline               Include a condensed agent timeline derived from taskOutput (default true)
```

### SEE ALSO

* [mcpchecker](mcpchecker.md)	 - MCP evaluation framework

