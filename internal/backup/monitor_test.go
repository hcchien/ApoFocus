package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeTrigger struct{ service string }

func (f *fakeTrigger) Kickstart(_ context.Context, service string) (string, error) {
	f.service = service
	return "com.apofocus." + service, nil
}

func TestMonitorReportsHealthyAndTriggersFixedServices(t *testing.T) {
	root, stateDirectory := t.TempDir(), t.TempDir()
	backupDirectory := filepath.Join(root, "postgres")
	if err := osMkdirAll(backupDirectory); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	archive := filepath.Join(backupDirectory, "apofocus.dump")
	if err := osWriteFile(archive); err != nil {
		t.Fatal(err)
	}
	state := State{Version: 1, LastBackupAt: &now, LastBackupPath: archive, LastVerifiedAt: &now, LastVerifiedPath: archive}
	statusPath := filepath.Join(stateDirectory, "status.json")
	if err := writeState(statusPath, state); err != nil {
		t.Fatal(err)
	}
	trigger := &fakeTrigger{}
	monitor := NewMonitorWithOptions(root, statusPath, "uuid", MonitorOptions{
		Now: func() time.Time { return now }, Controller: trigger,
		VolumeUUID: func(context.Context, string) (string, error) { return "uuid", nil },
		DiskSpace:  func(string) (uint64, uint64, error) { return 20 << 30, 100 << 30, nil },
	})
	report, err := monitor.Health(context.Background())
	if err != nil || report.Status != StatusHealthy || report.BackupCount != 1 {
		t.Fatalf("unexpected health: report=%+v err=%v", report, err)
	}
	if _, err := monitor.TriggerBackup(context.Background()); err != nil || trigger.service != "backup" {
		t.Fatalf("backup trigger failed: service=%q err=%v", trigger.service, err)
	}
	if _, err := monitor.TriggerVerify(context.Background()); err != nil || trigger.service != "backup-verify" {
		t.Fatalf("verify trigger failed: service=%q err=%v", trigger.service, err)
	}
}

func TestMonitorRejectsMissingOrMismatchedVolume(t *testing.T) {
	monitor := NewMonitorWithOptions(t.TempDir(), filepath.Join(t.TempDir(), "status.json"), "expected", MonitorOptions{
		VolumeUUID: func(context.Context, string) (string, error) { return "different", nil },
	})
	report, err := monitor.Health(context.Background())
	if err != nil || report.Status != StatusUnhealthy || report.RootAvailable {
		t.Fatalf("expected unavailable wrong volume: report=%+v err=%v", report, err)
	}
	if _, err := monitor.TriggerBackup(context.Background()); err == nil {
		t.Fatal("expected trigger to reject the wrong volume")
	}
}

// Small wrappers keep test setup errors explicit without hiding filesystem work in helpers.
func osMkdirAll(path string) error  { return os.MkdirAll(path, 0o700) }
func osWriteFile(path string) error { return os.WriteFile(path, []byte("archive"), 0o600) }
