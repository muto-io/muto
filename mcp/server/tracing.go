// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/muto-io/muto/core/tracing"
)

// WrapToolHandler returns a ToolHandlerFunc that records a span named name
// around every call to h.
func WrapToolHandler(name string, h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return tracing.Wrap(ctx, name, func(ctx context.Context) (*mcp.CallToolResult, error) {
			return h(ctx, req)
		})
	}
}
