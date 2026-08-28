package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/backup"
)

type BackupHealthOutput struct {
	Summary string              `json:"summary"`
	Report  backup.HealthReport `json:"report"`
}

type BackupActionOutput struct {
	Summary string               `json:"summary"`
	Result  backup.TriggerResult `json:"result"`
}

func addBackupTools(server *mcp.Server, operations backup.Operations) {
	closedWorld, nonDestructive, retentionDeletesExpired := false, false, true
	mcp.AddTool(server, &mcp.Tool{
		Name: "get_backup_health", Title: "Get PostgreSQL backup health",
		Description: "Check whether the configured external backup volume is mounted with the expected Volume UUID, inspect free space, latest completed PostgreSQL backup, restore-test age, current operation and last error.",
		Annotations: &mcp.ToolAnnotations{Title: "Get PostgreSQL backup health", ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input HealthInput) (*mcp.CallToolResult, BackupHealthOutput, error) {
		report, err := operations.Health(ctx)
		return nil, BackupHealthOutput{Summary: localizedBackupHealth(report, input.Locale), Report: report}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "run_backup", Title: "Start a PostgreSQL backup",
		Description: "Asynchronously kickstart only the fixed com.apofocus.backup LaunchAgent. The service validates the configured external Volume UUID, creates and validates a compressed PostgreSQL dump, atomically finalizes it and applies the retention policy. Poll get_backup_health for progress and completion.",
		Annotations: &mcp.ToolAnnotations{Title: "Start a PostgreSQL backup", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &retentionDeletesExpired, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input HealthInput) (*mcp.CallToolResult, BackupActionOutput, error) {
		result, err := operations.TriggerBackup(ctx)
		summary := localized(normalizedLocale(input.Locale), "備份服務已啟動；請用 get_backup_health 追蹤。", "The backup service was started; poll get_backup_health for progress.", "Der Sicherungsdienst wurde gestartet; den Fortschritt mit get_backup_health prüfen.")
		return nil, BackupActionOutput{Summary: summary, Result: result}, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Name: "verify_backup", Title: "Restore-test the latest PostgreSQL backup",
		Description: "Asynchronously kickstart only the fixed com.apofocus.backup-verify LaunchAgent. It restores the newest completed archive into a temporary database, checks required catalog relations, then removes the temporary database. It never replaces the live database. Poll get_backup_health for completion.",
		Annotations: &mcp.ToolAnnotations{Title: "Restore-test the latest PostgreSQL backup", ReadOnlyHint: false, IdempotentHint: true, DestructiveHint: &nonDestructive, OpenWorldHint: &closedWorld},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input HealthInput) (*mcp.CallToolResult, BackupActionOutput, error) {
		result, err := operations.TriggerVerify(ctx)
		summary := localized(normalizedLocale(input.Locale), "還原驗證服務已啟動；請用 get_backup_health 追蹤。", "The restore-test service was started; poll get_backup_health for progress.", "Der Wiederherstellungstest wurde gestartet; den Fortschritt mit get_backup_health prüfen.")
		return nil, BackupActionOutput{Summary: summary, Result: result}, err
	})
}

func localizedBackupHealth(report backup.HealthReport, locale string) string {
	switch report.Status {
	case backup.StatusHealthy:
		return localized(normalizedLocale(locale), "PostgreSQL 備份與實際還原驗證目前健康。", "PostgreSQL backups and restore tests are healthy.", "PostgreSQL-Sicherungen und Wiederherstellungstests sind aktuell.")
	case backup.StatusDegraded:
		return localized(normalizedLocale(locale), "備份服務可用，但備份或還原驗證需要留意。", "Backup is available, but a backup or restore test needs attention.", "Der Sicherungsdienst ist verfügbar, aber eine Sicherung oder ein Wiederherstellungstest benötigt Aufmerksamkeit.")
	default:
		return localized(normalizedLocale(locale), "備份磁碟或備份狀態不健康，請先查看詳細資料。", "The backup volume or backup state is unhealthy; inspect the report before retrying.", "Das Sicherungsvolume oder der Sicherungsstatus ist fehlerhaft; vor einem neuen Versuch den Bericht prüfen.")
	}
}
