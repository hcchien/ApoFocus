package maintenance

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
)

type fakePinger struct{ err error }

func (f fakePinger) PingContext(context.Context) error { return f.err }

type fakeJobs struct {
	byStatus map[string][]batch.Job
	err      error
}

func (f fakeJobs) List(_ context.Context, status string, _ int) ([]batch.Job, error) {
	return f.byStatus[status], f.err
}

type fakeController struct {
	restarted string
	after     func()
	err       error
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func healthClient(status func() int) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: status(), Status: http.StatusText(status()), Body: io.NopCloser(strings.NewReader("{}")), Header: make(http.Header)}, nil
	})}
}

func (f *fakeController) Restart(_ context.Context, service string) (string, error) {
	f.restarted = service
	if f.after != nil {
		f.after()
	}
	return "com.apofocus." + service, f.err
}

func TestCheckReportsFreshWorkerAndLocalServices(t *testing.T) {
	now := time.Now()
	root := t.TempDir()
	manager := NewManagerWithOptions(fakePinger{}, fakeJobs{byStatus: map[string][]batch.Job{
		"running": {{ID: "job", Status: "running", CreatedAt: now.Add(-time.Hour), HeartbeatAt: &now}},
	}}, root, []string{root}, "http://web", "http://embedding", Options{Now: func() time.Time { return now }, HTTPClient: healthClient(func() int { return http.StatusOK })})

	report, err := manager.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Database.Status != StatusHealthy || report.Web.Status != StatusHealthy || report.Embedding.Status != StatusHealthy {
		t.Fatalf("unexpected component health: %+v", report)
	}
	if report.Worker.Status != StatusHealthy || report.Worker.ActiveJobs != 1 || report.Worker.StaleJobs != 0 {
		t.Fatalf("unexpected worker health: %+v", report.Worker)
	}
	if len(report.Paths) != 1 || report.Paths[0].Status == StatusUnhealthy {
		t.Fatalf("unexpected path health: %+v", report.Paths)
	}
}

func TestCheckReportsStaleWorkerAndMissingLibrary(t *testing.T) {
	now := time.Now()
	stale := now.Add(-3 * time.Minute)
	manager := NewManagerWithOptions(fakePinger{}, fakeJobs{byStatus: map[string][]batch.Job{
		"running": {{ID: "job", Status: "running", CreatedAt: now.Add(-time.Hour), HeartbeatAt: &stale}},
	}}, t.TempDir()+"/missing", nil, "http://web", "http://embedding", Options{Now: func() time.Time { return now }, HTTPClient: healthClient(func() int { return http.StatusOK })})

	report, err := manager.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != StatusUnhealthy || report.Worker.StaleJobs != 1 || report.Paths[0].Status != StatusUnhealthy {
		t.Fatalf("expected unhealthy report, got %+v", report)
	}
}

func TestRepairUsesAllowlistedControllerAndWaitsForHealth(t *testing.T) {
	var healthy atomic.Bool
	client := healthClient(func() int {
		if !healthy.Load() {
			return http.StatusServiceUnavailable
		}
		return http.StatusOK
	})
	controller := &fakeController{after: func() { healthy.Store(true) }}
	manager := NewManagerWithOptions(fakePinger{}, fakeJobs{}, t.TempDir(), nil, "http://web", "http://embedding", Options{Controller: controller, HTTPClient: client})

	result, err := manager.Repair(context.Background(), "embedding")
	if err != nil {
		t.Fatal(err)
	}
	if controller.restarted != "embedding" || result.Label != "com.apofocus.embedding" || !result.Succeeded || result.Before.Status != StatusUnhealthy || result.After.Status != StatusHealthy {
		t.Fatalf("unexpected repair result: %+v", result)
	}
	if _, err := manager.Repair(context.Background(), "postgres; rm -rf /"); err == nil {
		t.Fatal("expected arbitrary service name to be rejected")
	}
}

func TestRepairAllowsFixedInitWorkerLaunchAgent(t *testing.T) {
	controller := &fakeController{}
	manager := NewManagerWithOptions(fakePinger{}, fakeJobs{}, t.TempDir(), nil, "http://web", "http://embedding", Options{Controller: controller, HTTPClient: healthClient(func() int { return http.StatusOK })})

	result, err := manager.Repair(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	if controller.restarted != "worker" || result.Label != "com.apofocus.worker" || !result.Succeeded || result.After.Status != StatusHealthy {
		t.Fatalf("unexpected worker repair result: %+v", result)
	}
}

func TestCheckSurfacesDatabaseFailure(t *testing.T) {
	manager := NewManagerWithOptions(fakePinger{err: errors.New("database offline")}, fakeJobs{}, t.TempDir(), nil, "http://web", "http://embedding", Options{HTTPClient: healthClient(func() int { return http.StatusOK })})
	report, err := manager.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.Database.Status != StatusUnhealthy || report.Status != StatusUnhealthy {
		t.Fatalf("expected database failure, got %+v", report)
	}
}
