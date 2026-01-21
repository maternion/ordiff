package main

import (
	"ordiff/cmd/cli"
	"ordiff/cmd/mcp"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	rootCmd := &cobra.Command{Use: "ordiff"}
	rootCmd.AddCommand(cli.IndexCmd, cli.ListCmd, cli.CompareCmd)
	rootCmd.AddCommand(mcp.McpCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
