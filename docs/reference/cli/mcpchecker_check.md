## mcpchecker check

Run an evaluation

### Synopsis

Run an evaluation using the specified eval configuration file.

```
mcpchecker check [eval-config-file] [flags]
```

### Options

```
      --cleanup-timeout string           Hard override cleanup timeout for ALL tasks (e.g., '2m')
      --default-cleanup-timeout string   Default cleanup timeout for tasks without their own (e.g., '2m')
      --default-task-timeout string      Default timeout for tasks without their own (e.g., '15m', '1h')
  -h, --help                             help for check
  -l, --label-selector string            Filter taskSets by labels (e.g., suite=k8s,suite=helm for OR; suite=k8s,difficulty=easy for AND)
      --mcp-config-file string           Path to MCP config file (overrides value in eval config)
  -o, --output string                    Output format (text, json) (default "text")
  -p, --parallel int                     Number of parallel workers for tasks marked as parallel (1 = sequential) (default 1)
  -r, --run string                       Regular expression to match task names to run (unanchored, like go test -run)
  -n, --runs int                         Number of times to run each task (for consistency testing) (default 1)
      --task-timeout string              Hard override timeout for ALL tasks (e.g., '15m', '1h')
  -v, --verbose                          Verbose output
```

### SEE ALSO

* [mcpchecker](mcpchecker.md)	 - MCP evaluation framework

