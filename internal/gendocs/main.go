package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/mcpchecker/mcpchecker/pkg/cli"
	"github.com/spf13/cobra/doc"
)

func main() {
	dir := "./docs/reference/cli"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Fatalf("failed to create directory %s: %v", dir, err)
	}

	cmd := cli.NewRootCmd()
	cmd.DisableAutoGenTag = true

	absDir, err := filepath.Abs(dir)
	if err != nil {
		log.Fatalf("failed to resolve path: %v", err)
	}

	if err := doc.GenMarkdownTree(cmd, absDir); err != nil {
		log.Fatalf("failed to generate docs: %v", err)
	}

	log.Printf("CLI docs generated in %s", absDir)
}
