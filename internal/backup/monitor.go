package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type MonitorOptions struct {
	Now        func() time.Time
	Controller TriggerController
	VolumeUUID func(context.Context, string) (string, error)
	DiskSpace  func(string) (uint64, uint64, error)
}

type Monitor struct {
	root               string
	statusPath         string
	expectedVolumeUUID string
	now                func() time.Time
	controller         TriggerController
	volumeUUID         func(context.Context, string) (string, error)
	diskSpace          func(string) (uint64, uint64, error)
}

func NewMonitor(root, statusPath, expectedVolumeUUID string) *Monitor {
	return NewMonitorWithOptions(root, statusPath, expectedVolumeUUID, MonitorOptions{})
}

func NewMonitorWithOptions(root, statusPath, expectedVolumeUUID string, options MonitorOptions) *Monitor {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	controller := options.Controller
	if controller == nil {
		controller = LaunchctlController{}
	}
	volumeUUID := options.VolumeUUID
	if volumeUUID == nil {
		volumeUUID = actualVolumeUUID
	}
	space := options.DiskSpace
	if space == nil {
		space = diskSpace
	}
	return &Monitor{
		root: strings.TrimSpace(root), statusPath: strings.TrimSpace(statusPath),
		expectedVolumeUUID: strings.TrimSpace(expectedVolumeUUID), now: now,
		controller: controller, volumeUUID: volumeUUID, diskSpace: space,
	}
}

func (m *Monitor) Health(ctx context.Context) (HealthReport, error) {
	now := m.now().UTC()
	report := HealthReport{Status: StatusUnhealthy, CheckedAt: now, Configured: m.root != "", Root: m.root}
	if m.root == "" {
		report.Detail = "backup root is not configured"
		return report, nil
	}
	if strings.HasPrefix(filepath.Clean(m.root), string(filepath.Separator)+"Volumes"+string(filepath.Separator)) && m.expectedVolumeUUID == "" {
		report.Detail = "external backup root requires a configured Volume UUID"
		return report, nil
	}
	info, err := os.Stat(m.root)
	if err != nil {
		report.Detail = "backup root is unavailable: " + err.Error()
		return report, nil
	}
	if !info.IsDir() {
		report.Detail = "backup root is not a directory"
		return report, nil
	}
	report.RootAvailable = true
	report.ExpectedVolumeUUID = m.expectedVolumeUUID
	if m.expectedVolumeUUID != "" {
		resolvedRoot, resolveErr := filepath.EvalSymlinks(m.root)
		if resolveErr != nil {
			report.RootAvailable = false
			report.Detail = "cannot resolve backup root: " + resolveErr.Error()
			return report, nil
		}
		actual, volumeErr := m.volumeUUID(ctx, resolvedRoot)
		if volumeErr != nil {
			report.RootAvailable = false
			report.Detail = "cannot identify backup volume: " + volumeErr.Error()
			return report, nil
		}
		report.ActualVolumeUUID = actual
		if !strings.EqualFold(actual, m.expectedVolumeUUID) {
			report.RootAvailable = false
			report.Detail = "backup volume UUID does not match the configured external volume"
			return report, nil
		}
	}
	spaceDetail := ""
	if free, total, spaceErr := m.diskSpace(m.root); spaceErr == nil {
		report.FreeBytes, report.TotalBytes = free, total
	} else {
		spaceDetail = "cannot inspect backup disk space: " + spaceErr.Error()
	}
	state, stateErr := readState(m.statusPath)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		report.Detail = "cannot read backup status: " + stateErr.Error()
		return report, nil
	}
	applyState(&report, state)
	files, scanErr := backupFiles(m.root)
	if scanErr != nil {
		report.Detail = "cannot inspect backup files: " + scanErr.Error()
		return report, nil
	}
	report.BackupCount = len(files)
	for _, file := range files {
		report.BackupBytes += file.Size()
	}

	report.Status, report.Detail = StatusHealthy, "scheduled PostgreSQL backups are current"
	if report.TotalBytes > 0 {
		freePercent := float64(report.FreeBytes) / float64(report.TotalBytes) * 100
		if report.FreeBytes < 1<<30 || freePercent < 1 {
			report.Status, report.Detail = StatusUnhealthy, "backup volume has critically low free space"
			return report, nil
		}
		if report.FreeBytes < 10<<30 || freePercent < 5 {
			report.Status, report.Detail = StatusDegraded, "backup volume has low free space"
		}
	} else if spaceDetail != "" {
		report.Status, report.Detail = StatusDegraded, spaceDetail
	}
	if report.RunningOperation != "" {
		if report.OperationStartedAt != nil && now.Sub(*report.OperationStartedAt) > 12*time.Hour {
			report.Status, report.Detail = StatusUnhealthy, "backup operation has been running for more than 12 hours"
		} else if report.Status == StatusHealthy {
			report.Detail = "backup operation is running"
		}
		return report, nil
	}
	if report.LastBackupAt == nil || report.BackupCount == 0 {
		report.Status, report.Detail = StatusDegraded, "no completed backup is available yet"
		if report.LastError != "" {
			report.Status, report.Detail = StatusUnhealthy, "no completed backup is available; the last operation failed"
		}
		return report, nil
	}
	backupAge := now.Sub(*report.LastBackupAt)
	if backupAge > 72*time.Hour {
		report.Status, report.Detail = StatusUnhealthy, "latest backup is more than 72 hours old"
	} else if backupAge > 36*time.Hour {
		report.Status, report.Detail = StatusDegraded, "latest backup is more than 36 hours old"
	}
	if report.LastVerifiedAt == nil || now.Sub(*report.LastVerifiedAt) > 35*24*time.Hour {
		if report.Status == StatusHealthy {
			report.Status = StatusDegraded
		}
		report.Detail = "latest backup has not passed a restore test in the last 35 days"
	}
	if report.LastError != "" && report.Status == StatusHealthy {
		report.Status, report.Detail = StatusDegraded, "the most recent backup operation reported an error"
	}
	return report, nil
}

func (m *Monitor) TriggerBackup(ctx context.Context) (TriggerResult, error) {
	return m.trigger(ctx, "backup")
}

func (m *Monitor) TriggerVerify(ctx context.Context) (TriggerResult, error) {
	return m.trigger(ctx, "backup-verify")
}

func (m *Monitor) trigger(ctx context.Context, service string) (TriggerResult, error) {
	report, err := m.Health(ctx)
	if err != nil {
		return TriggerResult{}, err
	}
	if !report.Configured || !report.RootAvailable {
		return TriggerResult{}, errors.New(report.Detail)
	}
	if service == "backup" && report.TotalBytes > 0 && report.FreeBytes < 1<<30 {
		return TriggerResult{}, errors.New("backup volume has critically low free space")
	}
	if report.RunningOperation != "" && (report.OperationStartedAt == nil || m.now().UTC().Sub(*report.OperationStartedAt) <= 12*time.Hour) {
		return TriggerResult{}, fmt.Errorf("backup operation %s is already running", report.RunningOperation)
	}
	if service == "backup-verify" && report.BackupCount == 0 {
		return TriggerResult{}, errors.New("no completed backup is available to verify")
	}
	label, err := m.controller.Kickstart(ctx, service)
	result := TriggerResult{Accepted: err == nil, Service: label, Action: "kickstart", At: m.now().UTC()}
	return result, err
}

func applyState(report *HealthReport, state State) {
	report.RunningOperation = state.RunningOperation
	report.OperationStartedAt = state.OperationStartedAt
	report.LastBackupAt = state.LastBackupAt
	report.LastBackupPath = state.LastBackupPath
	report.LastBackupBytes = state.LastBackupBytes
	report.LastVerifiedAt = state.LastVerifiedAt
	report.LastVerifiedPath = state.LastVerifiedPath
	report.LastError = state.LastError
}

func readState(path string) (State, error) {
	if strings.TrimSpace(path) == "" {
		return State{Version: 1}, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

func backupFiles(root string) ([]os.FileInfo, error) {
	entries, err := os.ReadDir(filepath.Join(root, "postgres"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	files := make([]os.FileInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".dump") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		files = append(files, info)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime().After(files[j].ModTime()) })
	return files, nil
}
