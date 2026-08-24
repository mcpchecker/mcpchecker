package cli

import (
	"github.com/spf13/cobra"
)

// NewRootCmd creates the root mcpchecker command
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "mcpchecker",
		Short: "MCP evaluation framework",
		Long: `mcpchecker is a framework for evaluating MCP agents against tasks.
It runs agents through defined tasks and validates their behavior using assertions.`,
		Version: version(),
	}

	// Add subcommands
	rootCmd.AddCommand(NewEvalCmd())
	rootCmd.AddCommand(NewInstallCmd())
	rootCmd.AddCommand(NewResultCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}

// Execute runs the root command
func Execute() error {
	return NewRootCmd().Execute()
}
