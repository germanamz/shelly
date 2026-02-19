// Package tools provides tool execution and MCP (Model Context Protocol) integration.
//
// It is organized into sub-packages:
//   - [github.com/germanamz/shelly/pkg/tools/toolbox] — Tool type and ToolBox orchestrator for registering, listing, and calling tools
//   - [github.com/germanamz/shelly/pkg/tools/mcpclient] — MCP client using the official MCP Go SDK for communicating with external MCP server processes
//   - [github.com/germanamz/shelly/pkg/tools/mcpserver] — MCP server using the official MCP Go SDK for exposing tools over the MCP protocol
//
// The toolbox sub-package is the foundation layer. Both mcpclient and mcpserver
// depend on toolbox for the Tool type but are independent of each other.
// The mcpclient and mcpserver packages are thin wrappers around the official
// MCP Go SDK (github.com/modelcontextprotocol/go-sdk).
package tools
