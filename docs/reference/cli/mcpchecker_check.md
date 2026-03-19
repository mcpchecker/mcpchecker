## mcpchecker check

Run an evaluation

### Synopsis

Run an evaluation using the specified eval configuration file.

```
mcpchecker check [eval-config-file] [flags]
```

### Options

```
  -h, --help                    help for check
  -l, --label-selector string   Filter taskSets by label (format: key=value, e.g., suite=kubernetes)
  -o, --output string           Output format (text, json) (default "text")
  -p, --parallel int            Number of parallel workers for tasks marked as parallel (1 = sequential) (default 1)
  -r, --run string              Regular expression to match task names to run (unanchored, like go test -run)
  -n, --runs int                Number of times to run each task (for consistency testing) (default 1)
  -v, --verbose                 Verbose output
```

### SEE ALSO

* [mcpchecker](mcpchecker.md)	 - MCP evaluation framework

