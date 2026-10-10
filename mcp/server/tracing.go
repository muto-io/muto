// SPDX-License-Identifier: Apache-2.0
package server

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

const instrumentationName = "github.com/muto-io/muto"

// WrapToolHandler returns a ToolHandlerFunc that records a span named name
// around every call to h. MCP tool handlers report tool-level failures by
// returning a *CallToolResult with IsError set and a nil Go error (see
// mcp.NewToolResultError), not by returning a non-nil error, so the span's
// error status must be derived from IsError in addition to err.
func WrapToolHandler(name string, h mcpserver.ToolHandlerFunc) mcpserver.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		tracer := otel.Tracer(instrumentationName)
		ctx, span := tracer.Start(ctx, name)
		defer span.End()

		result, err := h(ctx, req)
		switch {
		case err != nil:
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		case result != nil && result.IsError:
			span.SetStatus(codes.Error, toolResultErrorText(result))
		}
		return result, err
	}
}

// toolResultErrorText extracts the human-readable error message from a
// CallToolResult built by mcp.NewToolResultError, for use as the span's
// status description.
func toolResultErrorText(result *mcp.CallToolResult) string {
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return "tool call failed"
}
