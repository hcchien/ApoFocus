package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/hcchien/apofocus/internal/backup"
	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
	"github.com/hcchien/apofocus/internal/ingest"
	"github.com/hcchien/apofocus/internal/initjob"
	"github.com/hcchien/apofocus/internal/maintenance"
	"github.com/hcchien/apofocus/internal/mediaingest"
)

type fakeMediaStore struct{}

func (fakeMediaStore) ListMedia(context.Context, catalog.MediaFilter) (catalog.MediaPage, error) {
	return catalog.MediaPage{}, nil
}
func (fakeMediaStore) GetMedia(context.Context, string, string) (catalog.MediaAsset, error) {
	return catalog.MediaAsset{}, nil
}
func (fakeMediaStore) MediaFacets(context.Context, string) (catalog.MediaFacets, error) {
	return catalog.MediaFacets{}, nil
}
func (fakeMediaStore) SimilarMedia(context.Context, string, string, string, int) ([]catalog.SimilarMedia, error) {
	return nil, nil
}
func (fakeMediaStore) UpdateMedia(context.Context, string, string, catalog.MediaUpdate) (catalog.MediaAsset, error) {
	return catalog.MediaAsset{}, nil
}

type fakeMediaImporter struct{}

func (fakeMediaImporter) Inspect(context.Context, mediaingest.ImportRequest) (mediaingest.Inspection, error) {
	return mediaingest.Inspection{}, nil
}
func (fakeMediaImporter) Import(context.Context, mediaingest.ImportRequest) (mediaingest.ImportResult, error) {
	return mediaingest.ImportResult{}, nil
}

type fakeRelationStore struct{}

func (fakeRelationStore) ListRelationCatalog(context.Context) (catalog.RelationCatalog, error) {
	return catalog.RelationCatalog{Projects: []catalog.Project{}, Stories: []catalog.Story{}}, nil
}
func (fakeRelationStore) CreateProject(_ context.Context, description string) (catalog.Project, error) {
	return catalog.Project{ID: "project", Description: description}, nil
}
func (fakeRelationStore) UpdateProject(_ context.Context, id, description string) (catalog.Project, error) {
	return catalog.Project{ID: id, Description: description}, nil
}
func (fakeRelationStore) CreateStory(_ context.Context, description string) (catalog.Story, error) {
	return catalog.Story{ID: "story", Description: description}, nil
}
func (fakeRelationStore) UpdateStory(_ context.Context, id, description string) (catalog.Story, error) {
	return catalog.Story{ID: id, Description: description}, nil
}
func (fakeRelationStore) ReplaceProjectStories(_ context.Context, _ string, _ []string) ([]catalog.Story, error) {
	return []catalog.Story{}, nil
}
func (fakeRelationStore) BulkUpdateRelations(_ context.Context, input catalog.BulkRelationUpdate) (catalog.BulkRelationResult, error) {
	return catalog.BulkRelationResult{MediaType: input.MediaType, Operation: input.Operation, AssetCount: len(input.AssetIDs)}, nil
}

type fakeFolders struct{}

func (fakeFolders) List(context.Context) ([]folders.Collection, error) { return nil, nil }
func (fakeFolders) Create(_ context.Context, input folders.CreateInput) (folders.Collection, error) {
	return folders.Collection{ID: "collection", Name: input.Name, Kind: input.Kind}, nil
}
func (fakeFolders) AddPhotos(context.Context, string, []string) error  { return nil }
func (fakeFolders) PhotoIDs(context.Context, string) ([]string, error) { return nil, nil }

type fakeBatchJobs struct{ job batch.Job }

func (f *fakeBatchJobs) Create(context.Context, batch.CreateInput) (batch.Job, error) {
	return f.job, nil
}
func (f *fakeBatchJobs) List(context.Context, string, int) ([]batch.Job, error) {
	return []batch.Job{f.job}, nil
}
func (f *fakeBatchJobs) Get(context.Context, string) (batch.Job, error)           { return f.job, nil }
func (f *fakeBatchJobs) Items(context.Context, string, int) ([]batch.Item, error) { return nil, nil }
func (f *fakeBatchJobs) Cancel(context.Context, string) error                     { return nil }
func (f *fakeBatchJobs) Resume(context.Context, string) (batch.Job, error)        { return f.job, nil }

type fakeInitJobs struct{ run initjob.Run }

func (f *fakeInitJobs) Create(context.Context, initjob.CreateInput) (initjob.Run, error) {
	return f.run, nil
}
func (f *fakeInitJobs) Get(context.Context, string) (initjob.Run, error) { return f.run, nil }
func (f *fakeInitJobs) List(context.Context, string, int) ([]initjob.Run, error) {
	return []initjob.Run{f.run}, nil
}
func (f *fakeInitJobs) Items(context.Context, string, int) ([]initjob.Item, error) { return nil, nil }
func (f *fakeInitJobs) Pause(context.Context, string) error                        { return nil }
func (f *fakeInitJobs) Resume(context.Context, string) (initjob.Run, error)        { return f.run, nil }
func (f *fakeInitJobs) Cancel(context.Context, string) error                       { return nil }

type fakeMaintenance struct{ report maintenance.HealthReport }

func (f fakeMaintenance) Check(context.Context) (maintenance.HealthReport, error) {
	return f.report, nil
}

type fakeBackup struct{ report backup.HealthReport }

func (f fakeBackup) Health(context.Context) (backup.HealthReport, error) { return f.report, nil }
func (fakeBackup) TriggerBackup(context.Context) (backup.TriggerResult, error) {
	return backup.TriggerResult{Accepted: true, Service: "com.apofocus.backup", Action: "kickstart"}, nil
}
func (fakeBackup) TriggerVerify(context.Context) (backup.TriggerResult, error) {
	return backup.TriggerResult{Accepted: true, Service: "com.apofocus.backup-verify", Action: "kickstart"}, nil
}
func (f fakeMaintenance) Repair(_ context.Context, service string) (maintenance.RepairResult, error) {
	return maintenance.RepairResult{Service: service, Label: "com.apofocus." + service, Action: "restart", Succeeded: true}, nil
}

func TestFullServerAdvertisesCompleteToolset(t *testing.T) {
	ctx := context.Background()
	inbox, library := t.TempDir(), t.TempDir()
	manager, err := ingest.NewManager(library, []string{inbox}, noOpAnalyzer{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	jobs := &fakeBatchJobs{job: batch.Job{ID: "job", SourceRoot: inbox, Status: "running", DiscoveredCount: 10, ProcessedCount: 4, CreatedAt: now, HeartbeatAt: &now}}
	initJobs := &fakeInitJobs{run: initjob.Run{ID: "init", SourceRoot: inbox, Status: "cataloging", DiscoveredCount: 10, CatalogedCount: 4, CreatedAt: now, HeartbeatAt: &now}}
	health := maintenance.HealthReport{Status: maintenance.StatusHealthy, Database: maintenance.ComponentHealth{Status: maintenance.StatusHealthy}, Web: maintenance.ComponentHealth{Status: maintenance.StatusHealthy}, Embedding: maintenance.ComponentHealth{Status: maintenance.StatusHealthy}, Worker: maintenance.WorkerHealth{Status: maintenance.StatusHealthy}}
	server := NewWithOptions(Options{
		PhotoImporter: manager, MediaImporter: fakeMediaImporter{}, Photos: catalog.NewMemoryStore(), Media: fakeMediaStore{}, Relations: fakeRelationStore{},
		Folders: fakeFolders{}, BatchJobs: jobs, InitJobs: initJobs, Maintenance: fakeMaintenance{report: health},
		Backup:      fakeBackup{report: backup.HealthReport{Status: backup.StatusHealthy, Configured: true, RootAvailable: true}},
		ImportRoots: []string{inbox}, LibraryRoot: library,
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	wanted := []string{"get_photo_import_policy", "inspect_photo", "import_photo", "search_photos", "get_photo", "find_similar_photos", "update_photo_metadata", "search_media", "get_media", "find_similar_media", "update_media_metadata", "list_projects_and_stories", "create_project", "update_project", "create_story", "update_story", "update_project_story_relationships", "bulk_update_asset_relationships", "inspect_media", "import_media", "browse_folders", "create_collection", "add_photos_to_collection", "get_collection_photos", "create_batch_job", "get_batch_job", "list_batch_jobs", "wait_batch_job", "list_batch_items", "cancel_batch_job", "resume_batch_job", "create_init_run", "get_init_status", "list_init_runs", "list_init_items", "pause_init_run", "resume_init_run", "cancel_init_run", "get_system_health", "diagnose_batch_job", "repair_managed_service", "get_backup_health", "run_backup", "verify_backup"}
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		seen[tool.Name] = true
		if tool.InputSchema == nil {
			t.Errorf("%s has no input schema", tool.Name)
		}
	}
	if len(tools.Tools) != len(wanted) {
		t.Fatalf("expected %d tools, got %d", len(wanted), len(tools.Tools))
	}
	for _, name := range wanted {
		if !seen[name] {
			t.Errorf("missing tool %s", name)
		}
	}
	result, err := clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "list_projects_and_stories", Arguments: map[string]any{}})
	if err != nil || result.IsError || result.StructuredContent == nil {
		t.Fatalf("list_projects_and_stories failed: err=%v result=%+v", err, result)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "update_project_story_relationships", Arguments: map[string]any{"project_id": "project", "story_ids": []any{"story"}, "confirmed": true}})
	if err != nil || result.IsError {
		t.Fatalf("update_project_story_relationships failed: err=%v result=%+v", err, result)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "bulk_update_asset_relationships", Arguments: map[string]any{"media_type": "photo", "asset_ids": []any{"asset"}, "operation": "add", "apply_projects": true, "project_ids": []any{"project"}, "confirmed": true}})
	if err != nil || result.IsError {
		t.Fatalf("bulk_update_asset_relationships failed: err=%v result=%+v", err, result)
	}

	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "get_batch_job", Arguments: map[string]any{"job_id": "job", "locale": "de"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("get_batch_job returned an error: %+v", result.Content)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "diagnose_batch_job", Arguments: map[string]any{"job_id": "job", "locale": "zh-TW"}})
	if err != nil || result.IsError {
		t.Fatalf("diagnose_batch_job failed: err=%v result=%+v", err, result)
	}
	result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: "get_backup_health", Arguments: map[string]any{"locale": "de"}})
	if err != nil || result.IsError {
		t.Fatalf("get_backup_health failed: err=%v result=%+v", err, result)
	}
	for _, name := range []string{"run_backup", "verify_backup"} {
		result, err = clientSession.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"locale": "en"}})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: err=%v result=%+v", name, err, result)
		}
	}
}

func TestBatchStatusIncludesLocalizedRecoveryGuidance(t *testing.T) {
	failed := batchStatus(batch.Job{Status: "completed_with_errors", DiscoveredCount: 10, ProcessedCount: 10}, "zh-TW", true)
	if failed.StatusLabel != "完成，部分檔案失敗" || failed.Recovery != "manual_resume_available" || failed.ProgressPercent != 100 {
		t.Fatalf("unexpected localized failed status: %+v", failed)
	}
	running := batchStatus(batch.Job{Status: "running", DiscoveredCount: 10, ProcessedCount: 4}, "de-DE", false)
	if running.StatusLabel != "Wird verarbeitet" || running.Recovery != "automatic" || running.ProgressPercent != 40 {
		t.Fatalf("unexpected localized running status: %+v", running)
	}
}

func TestMaintenanceModeAdvertisesOnlyHealthAndRepair(t *testing.T) {
	ctx := context.Background()
	server := NewWithOptions(Options{Maintenance: fakeMaintenance{report: maintenance.HealthReport{Status: maintenance.StatusUnhealthy}}})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()

	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 2 || tools.Tools[0].Name != "get_system_health" || tools.Tools[1].Name != "repair_managed_service" {
		t.Fatalf("unexpected maintenance toolset: %+v", tools.Tools)
	}
}

func TestMaintenanceModeKeepsConfiguredBackupTools(t *testing.T) {
	ctx := context.Background()
	server := NewWithOptions(Options{
		Maintenance: fakeMaintenance{report: maintenance.HealthReport{Status: maintenance.StatusUnhealthy}},
		Backup:      fakeBackup{report: backup.HealthReport{Status: backup.StatusHealthy, Configured: true, RootAvailable: true}},
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "v1"}, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer clientSession.Close()
	tools, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	wanted := map[string]bool{"get_system_health": true, "repair_managed_service": true, "get_backup_health": true, "run_backup": true, "verify_backup": true}
	if len(tools.Tools) != len(wanted) {
		t.Fatalf("expected %d maintenance and backup tools, got %d", len(wanted), len(tools.Tools))
	}
	for _, tool := range tools.Tools {
		if !wanted[tool.Name] {
			t.Errorf("unexpected maintenance-mode tool %s", tool.Name)
		}
	}
}
