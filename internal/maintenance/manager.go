package maintenance

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
)

const staleHeartbeatAfter = 2 * time.Minute

type Options struct {
	HTTPClient *http.Client
	Controller Controller
	Now        func() time.Time
}

type Manager struct {
	database     Pinger
	jobs         BatchLister
	libraryRoot  string
	importRoots  []string
	webURL       string
	embeddingURL string
	httpClient   *http.Client
	controller   Controller
	now          func() time.Time
}

func NewManager(database Pinger, jobs BatchLister, libraryRoot string, importRoots []string, webURL, embeddingURL string) *Manager {
	return NewManagerWithOptions(database, jobs, libraryRoot, importRoots, webURL, embeddingURL, Options{})
}

func NewManagerWithOptions(database Pinger, jobs BatchLister, libraryRoot string, importRoots []string, webURL, embeddingURL string, options Options) *Manager {
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	controller := options.Controller
	if controller == nil {
		controller = LaunchctlController{}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &Manager{
		database: database, jobs: jobs, libraryRoot: libraryRoot, importRoots: append([]string(nil), importRoots...),
		webURL: strings.TrimRight(webURL, "/"), embeddingURL: strings.TrimRight(embeddingURL, "/"),
		httpClient: client, controller: controller, now: now,
	}
}

func (m *Manager) Check(ctx context.Context) (HealthReport, error) {
	report := HealthReport{CheckedAt: m.now().UTC(), Repairable: []string{"postgres", "web", "embedding"}}
	type componentResult struct {
		name   string
		health ComponentHealth
	}
	results := make(chan componentResult, 3)
	go func() { results <- componentResult{"database", m.checkDatabase(ctx)} }()
	go func() { results <- componentResult{"web", m.checkEndpoint(ctx, m.webURL)} }()
	go func() { results <- componentResult{"embedding", m.checkEndpoint(ctx, m.embeddingURL)} }()
	for range 3 {
		result := <-results
		switch result.name {
		case "database":
			report.Database = result.health
		case "web":
			report.Web = result.health
		case "embedding":
			report.Embedding = result.health
		}
	}

	report.Paths = append(report.Paths, checkPath("library", m.libraryRoot))
	seen := map[string]bool{filepath.Clean(m.libraryRoot): true}
	for _, root := range m.importRoots {
		clean := filepath.Clean(root)
		if strings.TrimSpace(root) == "" || seen[clean] {
			continue
		}
		seen[clean] = true
		report.Paths = append(report.Paths, checkPath("import", root))
	}
	report.Worker = m.checkWorker(ctx, report.Web)
	report.Status = overallStatus(report)
	return report, nil
}

func (m *Manager) Repair(ctx context.Context, service string) (RepairResult, error) {
	service = strings.ToLower(strings.TrimSpace(service))
	check := func(context.Context) ComponentHealth { return ComponentHealth{Status: StatusUnknown} }
	switch service {
	case "postgres":
		check = m.checkDatabase
	case "web":
		check = func(ctx context.Context) ComponentHealth { return m.checkEndpoint(ctx, m.webURL) }
	case "embedding":
		check = func(ctx context.Context) ComponentHealth { return m.checkEndpoint(ctx, m.embeddingURL) }
	default:
		return RepairResult{}, errors.New("service must be postgres, web, or embedding")
	}
	result := RepairResult{Service: service, Action: "restart", Before: check(ctx)}
	label, err := m.controller.Restart(ctx, service)
	result.Label = label
	if err != nil {
		result.After = check(ctx)
		return result, err
	}

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		result.After = check(ctx)
		if result.After.Status == StatusHealthy {
			result.Succeeded = true
			return result, nil
		}
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-deadline.C:
			return result, fmt.Errorf("%s did not become healthy after restart", service)
		case <-ticker.C:
		}
	}
}

func (m *Manager) checkDatabase(ctx context.Context) ComponentHealth {
	if m.database == nil {
		return ComponentHealth{Status: StatusUnknown, Detail: "database checker is not configured"}
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := m.database.PingContext(checkCtx); err != nil {
		return ComponentHealth{Status: StatusUnhealthy, Detail: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	return ComponentHealth{Status: StatusHealthy, Detail: "PostgreSQL is reachable", LatencyMS: time.Since(started).Milliseconds()}
}

func (m *Manager) checkEndpoint(ctx context.Context, baseURL string) ComponentHealth {
	if baseURL == "" {
		return ComponentHealth{Status: StatusUnknown, Detail: "service URL is not configured"}
	}
	started := time.Now()
	checkCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(checkCtx, http.MethodGet, baseURL+"/healthz", nil)
	if err != nil {
		return ComponentHealth{Status: StatusUnhealthy, Detail: err.Error()}
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return ComponentHealth{Status: StatusUnhealthy, Detail: err.Error(), LatencyMS: time.Since(started).Milliseconds()}
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ComponentHealth{Status: StatusUnhealthy, Detail: response.Status, LatencyMS: time.Since(started).Milliseconds()}
	}
	return ComponentHealth{Status: StatusHealthy, Detail: "health endpoint responded", LatencyMS: time.Since(started).Milliseconds()}
}

func (m *Manager) checkWorker(ctx context.Context, web ComponentHealth) WorkerHealth {
	result := WorkerHealth{Status: StatusUnknown, Detail: "batch queue is unavailable"}
	if m.jobs == nil {
		return result
	}
	var active []batch.Job
	for _, status := range []string{"scanning", "running"} {
		jobs, err := m.jobs.List(ctx, status, 100)
		if err != nil {
			result.Detail = err.Error()
			return result
		}
		active = append(active, jobs...)
	}
	pending, err := m.jobs.List(ctx, "pending", 100)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	result.ActiveJobs = len(active)
	result.PendingJobs = len(pending)
	now := m.now()
	for _, job := range active {
		if job.HeartbeatAt == nil || now.Sub(*job.HeartbeatAt) > staleHeartbeatAfter {
			result.StaleJobs++
		}
		if job.HeartbeatAt != nil && (result.LatestHeartbeat == nil || job.HeartbeatAt.After(*result.LatestHeartbeat)) {
			heartbeat := *job.HeartbeatAt
			result.LatestHeartbeat = &heartbeat
		}
	}
	oldPending := 0
	for _, job := range pending {
		if now.Sub(job.CreatedAt) > staleHeartbeatAfter {
			oldPending++
		}
	}
	if web.Status != StatusHealthy {
		result.Status, result.Detail = StatusUnhealthy, "Web process is unavailable; its embedded batch worker cannot run"
	} else if result.StaleJobs > 0 {
		result.Status, result.Detail = StatusUnhealthy, "one or more active jobs have a stale heartbeat"
	} else if oldPending > 0 {
		result.Status, result.Detail = StatusDegraded, "jobs have remained pending for more than two minutes"
	} else if result.ActiveJobs > 0 {
		result.Status, result.Detail = StatusHealthy, "worker is processing batch jobs"
	} else if result.PendingJobs > 0 {
		result.Status, result.Detail = StatusHealthy, "jobs are waiting to be claimed"
	} else {
		result.Status, result.Detail = StatusHealthy, "worker is idle"
	}
	return result
}

func checkPath(role, value string) PathHealth {
	result := PathHealth{Role: role, Path: value, Status: StatusUnhealthy}
	info, err := os.Stat(value)
	if err != nil {
		result.Detail = err.Error()
		return result
	}
	if !info.IsDir() {
		result.Detail = "path is not a directory"
		return result
	}
	free, total, err := diskSpace(value)
	if err != nil {
		result.Status, result.Detail = StatusDegraded, err.Error()
		return result
	}
	result.FreeBytes, result.TotalBytes = free, total
	if total > 0 {
		result.FreePercent = float64(free) / float64(total) * 100
	}
	result.Status, result.Detail = StatusHealthy, "path is available"
	if role == "library" && (free < 1<<30 || result.FreePercent < 1) {
		result.Status, result.Detail = StatusUnhealthy, "managed library has critically low free space"
	} else if role == "library" && (free < 10<<30 || result.FreePercent < 5) {
		result.Status, result.Detail = StatusDegraded, "managed library has low free space"
	}
	return result
}

func overallStatus(report HealthReport) string {
	status := StatusHealthy
	components := []string{report.Database.Status, report.Web.Status, report.Embedding.Status, report.Worker.Status}
	for _, path := range report.Paths {
		components = append(components, path.Status)
	}
	for _, component := range components {
		if component == StatusUnhealthy {
			return StatusUnhealthy
		}
		if component == StatusDegraded || component == StatusUnknown {
			status = StatusDegraded
		}
	}
	return status
}
