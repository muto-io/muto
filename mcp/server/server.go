package server

import (
	"context"
	"encoding/json"
	"fmt"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/muto-io/muto/core/scheduler"
	"github.com/muto-io/muto/mcp/tools"

	"github.com/mark3labs/mcp-go/mcp"
)

// MutoMCPServer wraps the mark3labs MCP server and exposes the Muto scheduler
// tools over the Model Context Protocol.
type MutoMCPServer struct {
	srv      *mcpserver.MCPServer
	handlers *tools.Handlers
}

// New creates a new MutoMCPServer wired with all 4 Muto scheduler tools.
func New(sched scheduler.Scheduler) *MutoMCPServer {
	srv := mcpserver.NewMCPServer("muto-scheduler", "0.1.0")
	h := tools.NewHandlers(sched)

	s := &MutoMCPServer{srv: srv, handlers: h}
	s.registerTools()
	return s
}

// ServeStdio starts the MCP server listening on os.Stdin / os.Stdout.
func (s *MutoMCPServer) ServeStdio() error {
	return mcpserver.ServeStdio(s.srv)
}

func (s *MutoMCPServer) registerTools() {
	s.srv.AddTool(scheduleAgentJobTool(), s.handleScheduleAgentJob)
	s.srv.AddTool(getJobStatusTool(), s.handleGetJobStatus)
	s.srv.AddTool(cancelJobTool(), s.handleCancelJob)
	s.srv.AddTool(listActiveAgentsTool(), s.handleListActiveAgents)
}

// ---- tool definitions -------------------------------------------------------

func scheduleAgentJobTool() mcp.Tool {
	return mcp.NewTool("schedule_agent_job",
		mcp.WithDescription("Schedule a new agent job on the Muto scheduler."),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("Unique identifier for the job."),
		),
		mcp.WithString("tenant_id",
			mcp.Required(),
			mcp.Description("Tenant that owns this job."),
		),
		mcp.WithString("image",
			mcp.Required(),
			mcp.Description("Container image for the agent worker."),
		),
		mcp.WithString("trigger_source",
			mcp.Required(),
			mcp.Description("Source that triggered this job (e.g. event topic)."),
		),
		mcp.WithNumber("ttl",
			mcp.Required(),
			mcp.Description("Time-to-live in seconds after job completion before cleanup."),
		),
	)
}

func getJobStatusTool() mcp.Tool {
	return mcp.NewTool("get_job_status",
		mcp.WithDescription("Retrieve the current status of an agent job."),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("Unique identifier of the job."),
		),
	)
}

func cancelJobTool() mcp.Tool {
	return mcp.NewTool("cancel_job",
		mcp.WithDescription("Cancel a running agent job."),
		mcp.WithString("job_id",
			mcp.Required(),
			mcp.Description("Unique identifier of the job to cancel."),
		),
	)
}

func listActiveAgentsTool() mcp.Tool {
	return mcp.NewTool("list_active_agents",
		mcp.WithDescription("List all active agent jobs for a tenant."),
		mcp.WithString("tenant_id",
			mcp.Required(),
			mcp.Description("Tenant whose active agents should be listed."),
		),
	)
}

// ---- tool handlers ----------------------------------------------------------

func (s *MutoMCPServer) handleScheduleAgentJob(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jobID, err := req.RequireString("job_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	tenantID, err := req.RequireString("tenant_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	image, err := req.RequireString("image")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	triggerSource, err := req.RequireString("trigger_source")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	ttlFloat := req.GetFloat("ttl", 0)
	ttl := int32(ttlFloat)

	if err := s.handlers.ScheduleAgentJob(ctx, jobID, tenantID, image, triggerSource, ttl); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("schedule_agent_job failed: %s", err.Error())), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q scheduled successfully.", jobID)), nil
}

func (s *MutoMCPServer) handleGetJobStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jobID, err := req.RequireString("job_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	status, err := s.handlers.GetJobStatus(ctx, jobID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("get_job_status failed: %s", err.Error())), nil
	}

	data, err := json.Marshal(status)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal status: %s", err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}

func (s *MutoMCPServer) handleCancelJob(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	jobID, err := req.RequireString("job_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := s.handlers.CancelJob(ctx, jobID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("cancel_job failed: %s", err.Error())), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Job %q cancelled successfully.", jobID)), nil
}

func (s *MutoMCPServer) handleListActiveAgents(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tenantID, err := req.RequireString("tenant_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	jobs, err := s.handlers.ListActiveAgents(ctx, tenantID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("list_active_agents failed: %s", err.Error())), nil
	}

	data, err := json.Marshal(jobs)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal jobs: %s", err.Error())), nil
	}
	return mcp.NewToolResultText(string(data)), nil
}
