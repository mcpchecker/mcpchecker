package cli

import (
	"github.com/spf13/cobra"
)

// NewConvertCmd creates the convert parent command
func NewConvertCmd() *cobra.Command {
	convertCmd := &cobra.Command{
		Use:   "convert",
		Short: "Commands for converting evaluation result files",
		Long:  `Commands for converting evaluation result files into other formats or reports.`,
	}

	convertCmd.AddCommand(NewJUnitCmd())

	return convertCmd
}
