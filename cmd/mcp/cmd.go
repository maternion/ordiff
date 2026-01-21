package mcp

import (
	"github.com/spf13/cobra"
)

var McpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Run ordiff as an MCP server",
	Long: `Starts ordiff as a Model Context Protocol server over stdio.

This is used by MCP clients like Claude Desktop or opencode to access
ordiff's tools for comparing GitHub releases.

Example:
  ordiff mcp`,
	Run: func(cmd *cobra.Command, args []string) {
		RunServer()
	},
}
