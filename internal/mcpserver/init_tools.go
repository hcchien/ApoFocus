package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/hcchien/apofocus/internal/initjob"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type CreateInitInput struct {
	SourceRoot string   `json:"source_root" jsonschema:"absolute folder or mounted volume inside APOFOCUS_IMPORT_ROOTS"`
	Project    string   `json:"project,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Recursive  *bool    `json:"recursive,omitempty"`
	Confirmed  bool     `json:"confirmed" jsonschema:"must be true after the user confirms reference cataloging; originals are not copied or moved"`
}
type InitIDInput struct {
	RunID     string `json:"run_id"`
	Confirmed bool   `json:"confirmed,omitempty"`
}
type ListInitInput struct {
	Status string `json:"status,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}
type InitStatusOutput struct {
	Run                       initjob.Run `json:"run"`
	PhaseProgress             float64     `json:"phaseProgressPercent"`
	OverallProgress           float64     `json:"overallProgressPercent"`
	EstimatedRemainingSeconds int64       `json:"estimatedRemainingSeconds,omitempty"`
	Recovery                  string      `json:"recovery"`
}

func addInitTools(server *mcp.Server, jobs InitJobs) {
	closed := false
	additive := false
	mcp.AddTool(server, &mcp.Tool{Name: "create_init_run", Title: "Create reference init run", Description: "Create a durable staged initialization run. Discovery and fast catalog happen first; originals remain in place; photo AI runs before slower video/audio AI.", Annotations: &mcp.ToolAnnotations{Title: "Create reference init run", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateInitInput) (*mcp.CallToolResult, initjob.Run, error) {
		if !input.Confirmed {
			return nil, initjob.Run{}, errors.New("confirmed must be true after the user approves the source root and reference mode")
		}
		recursive := true
		if input.Recursive != nil {
			recursive = *input.Recursive
		}
		run, err := jobs.Create(ctx, initjob.CreateInput{SourceRoot: strings.TrimSpace(input.SourceRoot), Project: strings.TrimSpace(input.Project), Tags: input.Tags, Recursive: recursive})
		return nil, run, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "get_init_status", Title: "Get init status", Description: "Inspect phase, progress, heartbeat, failures, estimated remaining time, and whether intervention is required.", Annotations: &mcp.ToolAnnotations{Title: "Get init status", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input InitIDInput) (*mcp.CallToolResult, InitStatusOutput, error) {
		run, err := jobs.Get(ctx, strings.TrimSpace(input.RunID))
		if err != nil {
			return nil, InitStatusOutput{}, err
		}
		return nil, initStatus(run), nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_init_runs", Title: "List init runs", Description: "List durable initialization runs and their current stages.", Annotations: &mcp.ToolAnnotations{Title: "List init runs", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListInitInput) (*mcp.CallToolResult, []initjob.Run, error) {
		runs, err := jobs.List(ctx, strings.TrimSpace(input.Status), bounded(input.Limit, 20, 1, 100))
		return nil, runs, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "list_init_items", Title: "List init items", Description: "List per-file catalog and AI status for diagnosis.", Annotations: &mcp.ToolAnnotations{Title: "List init items", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input InitIDInput) (*mcp.CallToolResult, []initjob.Item, error) {
		items, err := jobs.Items(ctx, strings.TrimSpace(input.RunID), 500)
		return nil, items, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "pause_init_run", Title: "Pause init run", Description: "Request a checkpointed pause. The worker stops after its current file and can resume later.", Annotations: &mcp.ToolAnnotations{Title: "Pause init run", ReadOnlyHint: false, IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input InitIDInput) (*mcp.CallToolResult, initjob.Run, error) {
		if err := jobs.Pause(ctx, strings.TrimSpace(input.RunID)); err != nil {
			return nil, initjob.Run{}, err
		}
		run, err := jobs.Get(ctx, input.RunID)
		return nil, run, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "resume_init_run", Title: "Resume init run", Description: "Requeue unfinished or stale init items; completed catalog and AI items are preserved.", Annotations: &mcp.ToolAnnotations{Title: "Resume init run", ReadOnlyHint: false, IdempotentHint: true, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input InitIDInput) (*mcp.CallToolResult, initjob.Run, error) {
		run, err := jobs.Resume(ctx, strings.TrimSpace(input.RunID))
		return nil, run, err
	})
	mcp.AddTool(server, &mcp.Tool{Name: "cancel_init_run", Title: "Cancel init run", Description: "Stop future work while preserving catalog records already created. confirmed must be true.", Annotations: &mcp.ToolAnnotations{Title: "Cancel init run", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &additive, OpenWorldHint: &closed}}, func(ctx context.Context, _ *mcp.CallToolRequest, input InitIDInput) (*mcp.CallToolResult, initjob.Run, error) {
		if !input.Confirmed {
			return nil, initjob.Run{}, errors.New("confirmed must be true")
		}
		if err := jobs.Cancel(ctx, strings.TrimSpace(input.RunID)); err != nil {
			return nil, initjob.Run{}, err
		}
		run, err := jobs.Get(ctx, input.RunID)
		return nil, run, err
	})
}

func initStatus(run initjob.Run) InitStatusOutput {
	total := max(run.DiscoveredCount, 1)
	phaseDone := 0
	phaseTotal := total
	overall := 0.0
	switch run.Status {
	case "scanning", "pending":
		overall = 0
	case "cataloging":
		phaseDone = run.CatalogedCount
		overall = 10 + 25*float64(phaseDone)/float64(phaseTotal)
	case "photo_ai":
		phaseDone = run.PhotoAICount
		phaseTotal = max(run.PhotoCount, 1)
		overall = 35 + 35*float64(phaseDone)/float64(phaseTotal)
	case "media_ai":
		phaseDone = run.MediaAICount
		phaseTotal = max(run.MediaCount, 1)
		overall = 70 + 30*float64(phaseDone)/float64(phaseTotal)
	case "paused", "failed", "cancelled":
		switch {
		case run.CatalogedCount < run.DiscoveredCount:
			phaseDone = run.CatalogedCount
			overall = 10 + 25*float64(phaseDone)/float64(phaseTotal)
		case run.PhotoAICount < run.PhotoCount:
			phaseDone, phaseTotal = run.PhotoAICount, max(run.PhotoCount, 1)
			overall = 35 + 35*float64(phaseDone)/float64(phaseTotal)
		default:
			phaseDone, phaseTotal = run.MediaAICount, max(run.MediaCount, 1)
			overall = 70 + 30*float64(phaseDone)/float64(phaseTotal)
		}
	case "completed", "completed_with_errors":
		phaseDone, phaseTotal, overall = total, total, 100
	}
	phase := 100 * float64(phaseDone) / float64(phaseTotal)
	remaining := int64(0)
	if run.StartedAt != nil && phaseDone > 0 && phaseDone < phaseTotal {
		elapsed := time.Since(*run.StartedAt).Seconds()
		remaining = int64(elapsed / float64(phaseDone) * float64(phaseTotal-phaseDone))
	}
	recovery := "automatic"
	if run.Status == "paused" {
		recovery = "resume_available"
	}
	if run.Status == "failed" {
		recovery = "diagnose_then_resume"
	}
	return InitStatusOutput{Run: run, PhaseProgress: min(100, phase), OverallProgress: min(100, overall), EstimatedRemainingSeconds: remaining, Recovery: recovery}
}
