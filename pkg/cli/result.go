package cli

import (
	"github.com/spf13/cobra"
)

// NewResultCmd creates the result parent command
func NewResultCmd() *cobra.Command {
	resultCmd := &cobra.Command{
		Use:   "result",
		Short: "Commands for inspecting and analyzing evaluation result files",
		Long: `Commands for inspecting and analyzing evaluation result files.

These commands operate on the JSON result files produced by 'mcpchecker check'.`,
	}

	resultCmd.AddCommand(NewViewCmd())
	resultCmd.AddCommand(NewVerifyCmd())
	resultCmd.AddCommand(NewSummaryCmd())
	resultCmd.AddCommand(NewDiffCmd())
	resultCmd.AddCommand(NewJUnitCmd())

	return resultCmd
}
