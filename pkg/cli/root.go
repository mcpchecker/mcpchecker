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
	}

	// Add subcommands
	rootCmd.AddCommand(NewEvalCmd())
	rootCmd.AddCommand(NewViewCmd())
	rootCmd.AddCommand(NewVerifyCmd())
	rootCmd.AddCommand(NewSummaryCmd())
	rootCmd.AddCommand(NewDiffCmd())
	rootCmd.AddCommand(NewVersionCmd())

	return rootCmd
}

// Execute runs the root command
func Execute() error {
	return NewRootCmd().Execute()
}
