package mcpserver

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/deepanalysis"
)

type CreateDeepAnalysisInput struct {
	PhotoIDs  []string `json:"photo_ids" jsonschema:"photo UUIDs selected from search results; maximum 500"`
	Force     bool     `json:"force,omitempty" jsonschema:"reanalyze even when the same model and prompt version already completed"`
	Confirmed bool     `json:"confirmed" jsonschema:"must be true after the user approves sending these photos to the locally configured large language model"`
}

type GetDeepAnalysisInput struct {
	JobID string `json:"job_id" jsonschema:"deep analysis job UUID"`
}

type GetPhotoDeepAnalysisInput struct {
	PhotoID string `json:"photo_id" jsonschema:"photo UUID"`
}

func addDeepAnalysisTools(server *mcp.Server, jobs DeepAnalysisJobs) {
	closedWorld, additive := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_photo_deep_analysis_job", Title: "使用大型語言模型深度分析照片",
		Description: "Queue structured caption, object, action, visible-text, and tag suggestions for 1-500 selected photos. Results are suggestions only and never overwrite human metadata or tags. confirmed must be true.",
		Annotations: &mcp.ToolAnnotations{Title: "使用大型語言模型深度分析照片", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateDeepAnalysisInput) (*mcp.CallToolResult, deepanalysis.Job, error) {
		if !input.Confirmed {
			return nil, deepanalysis.Job{}, errors.New("confirmed must be true after the user approves deep analysis")
		}
		job, err := jobs.Create(ctx, deepanalysis.CreateInput{PhotoIDs: input.PhotoIDs, Force: input.Force})
		return nil, job, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_photo_deep_analysis_job", Title: "查詢大型語言模型深度分析工作",
		Description: "Return the durable queue status and per-photo results for a deep analysis job.",
		Annotations: &mcp.ToolAnnotations{Title: "查詢大型語言模型深度分析工作", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetDeepAnalysisInput) (*mcp.CallToolResult, map[string]any, error) {
		job, err := jobs.Get(ctx, strings.TrimSpace(input.JobID))
		if err != nil {
			return nil, nil, err
		}
		items, err := jobs.Items(ctx, strings.TrimSpace(input.JobID), 500)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"job": job, "items": items}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "get_photo_deep_analysis", Title: "取得照片深度分析建議",
		Description: "Return the latest large-language-model suggestions for one photo without modifying its human metadata.",
		Annotations: &mcp.ToolAnnotations{Title: "取得照片深度分析建議", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input GetPhotoDeepAnalysisInput) (*mcp.CallToolResult, deepanalysis.PhotoAnalysis, error) {
		result, err := jobs.LatestForPhoto(ctx, strings.TrimSpace(input.PhotoID))
		return nil, result, err
	})
}
