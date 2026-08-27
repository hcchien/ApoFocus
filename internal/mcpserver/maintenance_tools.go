package mcpserver

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/maintenance"
)

type HealthInput struct {
	Locale string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for the human-readable summary"`
}

type HealthOutput struct {
	Summary string                   `json:"summary"`
	Report  maintenance.HealthReport `json:"report"`
}

type DiagnoseBatchInput struct {
	JobID  string `json:"job_id" jsonschema:"batch job UUID"`
	Locale string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for diagnosis and recommended actions"`
}

type ErrorSummary struct {
	Error string `json:"error"`
	Count int    `json:"count"`
}

type BatchDiagnosisOutput struct {
	Code               string                   `json:"code"`
	Summary            string                   `json:"summary"`
	RecommendedActions []string                 `json:"recommendedActions"`
	CanResume          bool                     `json:"canResume"`
	RequiresUserAction bool                     `json:"requiresUserAction"`
	Job                BatchStatusOutput        `json:"job"`
	Health             maintenance.HealthReport `json:"health"`
	Errors             []ErrorSummary           `json:"errors,omitempty"`
}

type RepairServiceInput struct {
	Service   string `json:"service" jsonschema:"managed service to restart: postgres, web, or embedding"`
	Confirmed bool   `json:"confirmed" jsonschema:"must be true after get_system_health or diagnose_batch_job shows that a restart is appropriate"`
	Locale    string `json:"locale,omitempty" jsonschema:"zh-TW, en, or de for the human-readable summary"`
}

type RepairServiceOutput struct {
	Summary string                   `json:"summary"`
	Result  maintenance.RepairResult `json:"result"`
}

func addMaintenanceTools(server *mcp.Server, operations maintenance.Checker, jobs BatchJobs) {
	closedWorld, nonDestructive := false, false
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_system_health", Title: "Get ApoFocus system health",
		Description: "Check local PostgreSQL, Web/Worker, embedding service, heartbeat freshness, managed library, import roots and disk capacity. Use this before attempting a repair.",
		Annotations: &mcp.ToolAnnotations{Title: "Get ApoFocus system health", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input HealthInput) (*mcp.CallToolResult, HealthOutput, error) {
		report, err := operations.Check(ctx)
		return nil, HealthOutput{Summary: localizedHealthSummary(report.Status, input.Locale), Report: report}, err
	})

	if jobs != nil {
		mcp.AddTool(server, &mcp.Tool{
			Name: "diagnose_batch_job", Title: "Diagnose a batch job",
			Description: "Combine persisted job progress, heartbeat, representative file errors, service health, storage availability and disk capacity into a safe recovery recommendation. Diagnose before restarting services or resuming a job.",
			Annotations: &mcp.ToolAnnotations{Title: "Diagnose a batch job", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
		}, func(ctx context.Context, _ *mcp.CallToolRequest, input DiagnoseBatchInput) (*mcp.CallToolResult, BatchDiagnosisOutput, error) {
			output, err := diagnoseBatch(ctx, operations, jobs, input)
			return nil, output, err
		})
	}

	mcp.AddTool(server, &mcp.Tool{
		Name: "repair_managed_service", Title: "Restart an ApoFocus managed service",
		Description: "Restart only the allowlisted com.apofocus.postgres, com.apofocus.web or com.apofocus.embedding macOS LaunchAgent, then wait for its local health check. confirmed must be true after diagnosis. This tool cannot execute arbitrary shell commands and does not resume terminal jobs automatically.",
		Annotations: &mcp.ToolAnnotations{Title: "Restart an ApoFocus managed service", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input RepairServiceInput) (*mcp.CallToolResult, RepairServiceOutput, error) {
		if !input.Confirmed {
			return nil, RepairServiceOutput{}, errors.New("confirmed must be true after checking system health or diagnosing the batch job")
		}
		result, err := operations.Repair(ctx, input.Service)
		summary := localized(normalizedLocale(input.Locale), "服務已重新啟動並通過健康檢查。", "The service was restarted and passed its health check.", "Der Dienst wurde neu gestartet und hat die Zustandsprüfung bestanden.")
		if err != nil {
			summary = localized(normalizedLocale(input.Locale), "服務修復未完成，請查看回傳錯誤。", "Service repair did not complete; inspect the returned error.", "Die Dienstreparatur wurde nicht abgeschlossen; bitte den zurückgegebenen Fehler prüfen.")
		}
		return nil, RepairServiceOutput{Summary: summary, Result: result}, err
	})
}

func diagnoseBatch(ctx context.Context, operations maintenance.Checker, jobs BatchJobs, input DiagnoseBatchInput) (BatchDiagnosisOutput, error) {
	job, err := jobs.Get(ctx, strings.TrimSpace(input.JobID))
	if err != nil {
		return BatchDiagnosisOutput{}, err
	}
	health, err := operations.Check(ctx)
	if err != nil {
		return BatchDiagnosisOutput{}, err
	}
	items, itemsErr := jobs.Items(ctx, job.ID, 1000)
	if itemsErr != nil {
		return BatchDiagnosisOutput{}, itemsErr
	}
	output := BatchDiagnosisOutput{Job: batchStatus(job, input.Locale, true), Health: health, Errors: summarizedErrors(items)}
	locale := normalizedLocale(input.Locale)
	set := func(code, zh, en, de string, canResume, userAction bool, actions ...string) BatchDiagnosisOutput {
		output.Code, output.Summary = code, localized(locale, zh, en, de)
		output.CanResume, output.RequiresUserAction, output.RecommendedActions = canResume, userAction, actions
		return output
	}
	if _, err := os.Stat(job.SourceRoot); err != nil {
		return set("source_unavailable", "來源資料夾或外接硬碟目前無法存取。", "The source folder or external volume is unavailable.", "Der Quellordner oder externe Datenträger ist nicht verfügbar.", false, true,
			localized(locale, "請接回外接硬碟或修正 macOS 資料夾權限，再重新診斷。", "Reconnect the external volume or fix macOS folder permission, then diagnose again.", "Externen Datenträger wieder anschließen oder macOS-Ordnerrechte korrigieren und erneut diagnostizieren.")), nil
	}
	if library := healthPath(health.Paths, "library"); library != nil && library.Status == maintenance.StatusUnhealthy {
		return set("library_unavailable", "Managed library 無法使用或剩餘空間嚴重不足。", "The managed library is unavailable or critically low on space.", "Die verwaltete Mediathek ist nicht verfügbar oder hat kritisch wenig Speicherplatz.", false, true,
			localized(locale, "請重新掛載 library 或釋放空間，再重新診斷。", "Remount the library or free disk space, then diagnose again.", "Mediathek erneut einbinden oder Speicherplatz freigeben und erneut diagnostizieren.")), nil
	}
	if health.Database.Status != maintenance.StatusHealthy {
		return set("database_unhealthy", "PostgreSQL 無法正常連線。", "PostgreSQL is not reachable.", "PostgreSQL ist nicht erreichbar.", false, false,
			localized(locale, "呼叫 repair_managed_service，service 設為 postgres；資料庫健康後重新連線 MCP。", "Call repair_managed_service for postgres; reconnect MCP after the database is healthy.", "repair_managed_service für postgres aufrufen und MCP nach Wiederherstellung der Datenbank neu verbinden.")), nil
	}
	if health.Web.Status != maintenance.StatusHealthy {
		return set("web_worker_unhealthy", "Web process 無回應，因此內嵌的 Batch Worker 也無法工作。", "The Web process is unavailable, so its embedded batch worker cannot run.", "Der Webprozess ist nicht verfügbar; daher kann der eingebettete Batch-Worker nicht arbeiten.", false, false,
			localized(locale, "呼叫 repair_managed_service，service 設為 web；健康後由 worker 自動接手 stale job。", "Call repair_managed_service for web; once healthy, the worker automatically reclaims stale jobs.", "repair_managed_service für web aufrufen; danach übernimmt der Worker veraltete Aufträge automatisch.")), nil
	}
	if health.Embedding.Status != maintenance.StatusHealthy {
		return set("embedding_unhealthy", "本機 embedding／媒體辨識服務無回應。", "The local embedding and media analysis service is unavailable.", "Der lokale Embedding- und Medienanalysedienst ist nicht verfügbar.", false, false,
			localized(locale, "呼叫 repair_managed_service，service 設為 embedding；確認健康後再 resume 終止的 job。", "Call repair_managed_service for embedding; after it is healthy, resume a terminal job if needed.", "repair_managed_service für embedding aufrufen; danach einen beendeten Auftrag bei Bedarf fortsetzen.")), nil
	}
	if job.Status == "scanning" || job.Status == "running" {
		if job.HeartbeatAt == nil || time.Since(*job.HeartbeatAt) > 2*time.Minute {
			return set("worker_heartbeat_stale", "Batch heartbeat 已超過兩分鐘未更新。", "The batch heartbeat has been stale for more than two minutes.", "Der Batch-Heartbeat wurde seit mehr als zwei Minuten nicht aktualisiert.", true, false,
				localized(locale, "Web 目前健康；稍候讓 worker 自動接手，若仍無進度再重啟 web。", "Web is healthy; allow the worker to reclaim the job, then restart web only if progress remains stalled.", "Web ist verfügbar; zunächst die automatische Übernahme abwarten und web nur bei weiterem Stillstand neu starten.")), nil
		}
		return set("processing_normally", "Worker heartbeat 正常，工作仍在處理中，不需要修復或 resume。", "The worker heartbeat is fresh and processing is normal; no repair or resume is needed.", "Der Worker-Heartbeat ist aktuell; keine Reparatur oder Fortsetzung ist nötig.", false, false), nil
	}
	switch job.Status {
	case "completed":
		return set("completed", "Batch 已成功完成。", "The batch completed successfully.", "Der Batch wurde erfolgreich abgeschlossen.", false, false), nil
	case "failed", "cancelled", "completed_with_errors":
		return set("resume_available", "依賴服務目前健康，可以只重試未完成或失敗項目。", "Dependencies are healthy; unfinished or failed items can be retried safely.", "Die Dienste sind verfügbar; unvollständige oder fehlgeschlagene Elemente können sicher erneut versucht werden.", true, false,
			localized(locale, "確認錯誤原因已排除後呼叫 resume_batch_job。", "After confirming the cause is resolved, call resume_batch_job.", "Nach Behebung der Ursache resume_batch_job aufrufen.")), nil
	case "pending":
		if time.Since(job.CreatedAt) > 2*time.Minute {
			return set("pending_too_long", "工作已等待超過兩分鐘，Worker 可能沒有正常 claim queue。", "The job has remained pending for more than two minutes; the worker may not be claiming the queue.", "Der Auftrag wartet seit mehr als zwei Minuten; möglicherweise übernimmt der Worker die Warteschlange nicht.", false, false,
				localized(locale, "重新確認健康狀態；若仍停滯，重啟 web service。", "Check health again; restart the web service if it remains stalled.", "Zustand erneut prüfen und bei weiterem Stillstand den web-Dienst neu starten.")), nil
		}
		return set("waiting", "工作正在等待 Worker claim。", "The job is waiting for the worker to claim it.", "Der Auftrag wartet auf die Übernahme durch den Worker.", false, false), nil
	default:
		return set("unknown", "無法判斷 Batch 狀態。", "The batch state could not be classified.", "Der Batch-Zustand konnte nicht klassifiziert werden.", false, false), nil
	}
}

func healthPath(paths []maintenance.PathHealth, role string) *maintenance.PathHealth {
	for index := range paths {
		if paths[index].Role == role {
			return &paths[index]
		}
	}
	return nil
}

func summarizedErrors(items []batch.Item) []ErrorSummary {
	counts := map[string]int{}
	for _, item := range items {
		if item.Status == "failed" && strings.TrimSpace(item.Error) != "" {
			counts[item.Error]++
		}
	}
	result := make([]ErrorSummary, 0, len(counts))
	for message, count := range counts {
		result = append(result, ErrorSummary{Error: message, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Error < result[j].Error
		}
		return result[i].Count > result[j].Count
	})
	if len(result) > 10 {
		result = result[:10]
	}
	return result
}

func localizedHealthSummary(status, locale string) string {
	switch status {
	case maintenance.StatusHealthy:
		return localized(normalizedLocale(locale), "ApoFocus 與 Batch Worker 目前健康。", "ApoFocus and its batch worker are healthy.", "ApoFocus und der Batch-Worker sind verfügbar.")
	case maintenance.StatusDegraded:
		return localized(normalizedLocale(locale), "ApoFocus 可以回應，但有項目需要留意。", "ApoFocus is responding, but one or more components need attention.", "ApoFocus antwortet, aber mindestens eine Komponente benötigt Aufmerksamkeit.")
	default:
		return localized(normalizedLocale(locale), "ApoFocus 有無法使用的元件，請先診斷再修復。", "One or more ApoFocus components are unavailable; diagnose before repairing.", "Mindestens eine ApoFocus-Komponente ist nicht verfügbar; vor der Reparatur diagnostizieren.")
	}
}
