package mcpserver

import (
	"context"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/batch"
)

type CreateBatchInput struct {
	SourceRoot string   `json:"source_root" jsonschema:"absolute folder or mounted volume path inside APOFOCUS_IMPORT_ROOTS"`
	Project    string   `json:"project,omitempty" jsonschema:"shared project; may be empty"`
	Tags       []string `json:"tags,omitempty" jsonschema:"shared tags applied to every imported item; maximum 20"`
	Recursive  *bool    `json:"recursive,omitempty" jsonschema:"include subfolders; defaults to true"`
	AutoTags   *bool    `json:"auto_tags,omitempty" jsonschema:"generate local model tags; defaults to true"`
	MediaTypes []string `json:"media_types,omitempty" jsonschema:"photo, video, and/or audio; defaults to all three"`
	Locale     string   `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for status labels"`
}

type BatchIDInput struct {
	JobID  string `json:"job_id" jsonschema:"batch job UUID"`
	Locale string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for status labels"`
}

type BatchItemsInput struct {
	JobID string `json:"job_id" jsonschema:"batch job UUID"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum items from 1 to 1000; defaults to 200"`
}

type ListBatchJobsInput struct {
	Status string `json:"status,omitempty" jsonschema:"optional pending, scanning, running, completed, completed_with_errors, failed, or cancelled filter"`
	Limit  int    `json:"limit,omitempty" jsonschema:"most recent jobs from 1 to 100; defaults to 20"`
	Locale string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for status labels"`
}

type BatchListOutput struct {
	Jobs []BatchStatusOutput `json:"jobs"`
}

type WaitBatchInput struct {
	JobID               string `json:"job_id" jsonschema:"batch job UUID"`
	AfterStatus         string `json:"after_status,omitempty" jsonschema:"last status observed by the caller"`
	AfterProcessedCount *int   `json:"after_processed_count,omitempty" jsonschema:"last processed count observed by the caller"`
	WaitSeconds         int    `json:"wait_seconds,omitempty" jsonschema:"long-poll duration from 1 to 30 seconds; defaults to 20"`
	Locale              string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for status labels"`
}

type BatchStatusOutput struct {
	Job                 batch.Job `json:"job"`
	StatusLabel         string    `json:"statusLabel"`
	ProgressPercent     int       `json:"progressPercent"`
	Changed             bool      `json:"changed"`
	Recovery            string    `json:"recovery"`
	RecoveryDescription string    `json:"recoveryDescription"`
}

type BatchItemsOutput struct {
	JobID string       `json:"jobId"`
	Items []batch.Item `json:"items"`
}

func addBatchTools(server *mcp.Server, jobs BatchJobs) {
	closedWorld, additive, nonDestructive := false, false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "create_batch_job", Title: "Create batch job", Description: "Persist a local sequential import job for a folder or mounted volume. Returns immediately; use get_batch_job or wait_batch_job to monitor it. The PostgreSQL-backed queue survives client disconnects and process restarts.",
		Annotations: &mcp.ToolAnnotations{Title: "Create batch job", ReadOnlyHint: false, IdempotentHint: false, DestructiveHint: &additive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input CreateBatchInput) (*mcp.CallToolResult, BatchStatusOutput, error) {
		recursive, autoTags := true, true
		if input.Recursive != nil {
			recursive = *input.Recursive
		}
		if input.AutoTags != nil {
			autoTags = *input.AutoTags
		}
		job, err := jobs.Create(ctx, batch.CreateInput{SourceRoot: input.SourceRoot, Project: input.Project, Tags: input.Tags, Recursive: recursive, AutoTags: autoTags, MediaTypes: input.MediaTypes})
		return nil, batchStatus(job, input.Locale, true), err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_batch_job", Title: "Get batch status", Description: "Return current persisted counts, status, active path, error and recovery guidance for a batch job.",
		Annotations: &mcp.ToolAnnotations{Title: "Get batch status", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BatchIDInput) (*mcp.CallToolResult, BatchStatusOutput, error) {
		job, err := jobs.Get(ctx, strings.TrimSpace(input.JobID))
		return nil, batchStatus(job, input.Locale, true), err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_batch_jobs", Title: "List batch jobs", Description: "List recent persisted batch jobs, optionally filtered by status, so monitoring can resume even when the caller no longer has a job ID in context.",
		Annotations: &mcp.ToolAnnotations{Title: "List batch jobs", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input ListBatchJobsInput) (*mcp.CallToolResult, BatchListOutput, error) {
		jobsList, err := jobs.List(ctx, input.Status, bounded(input.Limit, 20, 1, 100))
		if err != nil {
			return nil, BatchListOutput{}, err
		}
		result := make([]BatchStatusOutput, 0, len(jobsList))
		for _, job := range jobsList {
			result = append(result, batchStatus(job, input.Locale, true))
		}
		return nil, BatchListOutput{Jobs: result}, nil
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "wait_batch_job", Title: "Wait for batch progress", Description: "Long-poll PostgreSQL for up to 30 seconds and return when status or processed count changes. Repeated bounded calls provide continuous monitoring without an HTTP or MCP call that runs for the whole batch.",
		Annotations: &mcp.ToolAnnotations{Title: "Wait for batch progress", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input WaitBatchInput) (*mcp.CallToolResult, BatchStatusOutput, error) {
		output, err := waitForBatch(ctx, jobs, input)
		return nil, output, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "list_batch_items", Title: "List batch items", Description: "List per-file status, imported photo or media IDs, and errors for a persisted batch job.",
		Annotations: &mcp.ToolAnnotations{Title: "List batch items", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BatchItemsInput) (*mcp.CallToolResult, BatchItemsOutput, error) {
		items, err := jobs.Items(ctx, strings.TrimSpace(input.JobID), bounded(input.Limit, 200, 1, 1000))
		return nil, BatchItemsOutput{JobID: input.JobID, Items: items}, err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "cancel_batch_job", Title: "Cancel batch job", Description: "Request cooperative cancellation. The worker finishes its current atomic file step and then marks the job cancelled.",
		Annotations: &mcp.ToolAnnotations{Title: "Cancel batch job", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BatchIDInput) (*mcp.CallToolResult, BatchStatusOutput, error) {
		if err := jobs.Cancel(ctx, strings.TrimSpace(input.JobID)); err != nil {
			return nil, BatchStatusOutput{}, err
		}
		job, err := jobs.Get(ctx, strings.TrimSpace(input.JobID))
		return nil, batchStatus(job, input.Locale, true), err
	})
	mcp.AddTool(server, &mcp.Tool{
		Name: "resume_batch_job", Title: "Resume batch job", Description: "Requeue a stale, failed, cancelled, or partially failed job. Succeeded items remain succeeded; running and failed items return to pending. A live job with a recent heartbeat is rejected to prevent duplicate processing.",
		Annotations: &mcp.ToolAnnotations{Title: "Resume batch job", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input BatchIDInput) (*mcp.CallToolResult, BatchStatusOutput, error) {
		job, err := jobs.Resume(ctx, strings.TrimSpace(input.JobID))
		return nil, batchStatus(job, input.Locale, true), err
	})
}

func waitForBatch(ctx context.Context, jobs BatchJobs, input WaitBatchInput) (BatchStatusOutput, error) {
	wait := time.Duration(bounded(input.WaitSeconds, 20, 1, 30)) * time.Second
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := jobs.Get(ctx, strings.TrimSpace(input.JobID))
		if err != nil {
			return BatchStatusOutput{}, err
		}
		changed := input.AfterStatus == "" && input.AfterProcessedCount == nil
		changed = changed || (input.AfterStatus != "" && job.Status != input.AfterStatus)
		changed = changed || (input.AfterProcessedCount != nil && job.ProcessedCount != *input.AfterProcessedCount)
		if changed || job.Terminal() {
			return batchStatus(job, input.Locale, changed), nil
		}
		select {
		case <-ctx.Done():
			return BatchStatusOutput{}, ctx.Err()
		case <-deadline.C:
			return batchStatus(job, input.Locale, false), nil
		case <-ticker.C:
		}
	}
}

func batchStatus(job batch.Job, locale string, changed bool) BatchStatusOutput {
	percent := 0
	if job.DiscoveredCount > 0 {
		percent = int(float64(job.ProcessedCount) / float64(job.DiscoveredCount) * 100)
		if percent > 100 {
			percent = 100
		}
	}
	recovery := "automatic"
	description := localized(normalizedLocale(locale), "若 worker 中斷，heartbeat stale 兩分鐘後會自動接續。", "If the worker stops, the job is reclaimed automatically after its heartbeat is stale for two minutes.", "Wenn der Worker stoppt, wird der Auftrag nach zwei Minuten ohne Heartbeat automatisch übernommen.")
	if job.Status == "failed" || job.Status == "cancelled" || job.Status == "completed_with_errors" {
		recovery = "manual_resume_available"
		description = localized(normalizedLocale(locale), "可呼叫 resume_batch_job，只重試未完成或失敗項目。", "Call resume_batch_job to retry only unfinished or failed items.", "Mit resume_batch_job werden nur unvollständige oder fehlgeschlagene Elemente erneut versucht.")
	} else if job.Status == "completed" {
		recovery, description = "none", localized(normalizedLocale(locale), "工作已完成，不需要 resume。", "The job is complete and does not need to resume.", "Der Auftrag ist abgeschlossen und muss nicht fortgesetzt werden.")
	}
	return BatchStatusOutput{Job: job, StatusLabel: localizedBatchStatus(job.Status, locale), ProgressPercent: percent, Changed: changed, Recovery: recovery, RecoveryDescription: description}
}

func localizedBatchStatus(status, locale string) string {
	labels := map[string][3]string{
		"pending": {"等待處理", "Waiting", "Wartet"}, "scanning": {"掃描媒體檔案", "Scanning media", "Medien werden gescannt"},
		"running": {"處理中", "Running", "Wird verarbeitet"}, "completed": {"處理完成", "Completed", "Abgeschlossen"},
		"completed_with_errors": {"完成，部分檔案失敗", "Completed with errors", "Mit Fehlern abgeschlossen"},
		"failed":                {"工作失敗", "Failed", "Fehlgeschlagen"}, "cancelled": {"已停止", "Cancelled", "Abgebrochen"},
	}
	label, ok := labels[status]
	if !ok {
		return status
	}
	return localized(normalizedLocale(locale), label[0], label[1], label[2])
}
