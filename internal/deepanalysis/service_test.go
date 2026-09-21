package deepanalysis

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeRepository struct {
	created   CreateInput
	model     string
	prompt    string
	job       Job
	items     []Item
	completed json.RawMessage
}

func (r *fakeRepository) Create(_ context.Context, input CreateInput, model, prompt string) (Job, error) {
	r.created, r.model, r.prompt = input, model, prompt
	return Job{ID: "job", SelectedCount: len(input.PhotoIDs)}, nil
}
func (r *fakeRepository) Get(context.Context, string) (Job, error)           { return r.job, nil }
func (r *fakeRepository) Items(context.Context, string, int) ([]Item, error) { return r.items, nil }
func (r *fakeRepository) LatestForPhoto(context.Context, string) (PhotoAnalysis, error) {
	return PhotoAnalysis{}, nil
}
func (r *fakeRepository) Cancel(context.Context, string) error { return nil }
func (r *fakeRepository) ClaimNext(context.Context) (Job, bool, error) {
	if r.job.ID == "" {
		return Job{}, false, nil
	}
	job := r.job
	r.job = Job{}
	return job, true, nil
}
func (r *fakeRepository) NextItem(context.Context, string) (Item, bool, error) {
	if len(r.items) == 0 {
		return Item{}, false, nil
	}
	item := r.items[0]
	r.items = r.items[1:]
	return item, true, nil
}
func (r *fakeRepository) CompleteItem(_ context.Context, _ Job, _ Item, result json.RawMessage, err error) error {
	if err != nil {
		return err
	}
	r.completed = result
	return nil
}
func (r *fakeRepository) Heartbeat(context.Context, string) (bool, error) { return false, nil }
func (r *fakeRepository) Finish(context.Context, string, error) error     { return nil }

func TestServiceCreateCleansDuplicateIDsAndUsesDefaults(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository, "", "")
	job, err := service.Create(context.Background(), CreateInput{PhotoIDs: []string{" a ", "a", "b"}})
	if err != nil {
		t.Fatal(err)
	}
	if job.SelectedCount != 2 {
		t.Fatalf("selected count = %d", job.SelectedCount)
	}
	if len(repository.created.PhotoIDs) != 2 || repository.created.PhotoIDs[0] != "a" {
		t.Fatalf("cleaned IDs = %#v", repository.created.PhotoIDs)
	}
	if repository.model != DefaultModel || repository.prompt != DefaultPromptVersion {
		t.Fatalf("defaults = %q %q", repository.model, repository.prompt)
	}
}

func TestServiceCreateRejectsEmptySelection(t *testing.T) {
	_, err := NewService(&fakeRepository{}, "", "").Create(context.Background(), CreateInput{})
	if err == nil {
		t.Fatal("expected an error")
	}
}

type fakeAnalyzer struct {
	result json.RawMessage
	err    error
}

func (a fakeAnalyzer) Analyze(context.Context, string) (json.RawMessage, error) {
	return a.result, a.err
}

func TestWorkerProcessesOnePhoto(t *testing.T) {
	repository := &fakeRepository{
		job:   Job{ID: "job", Model: DefaultModel, PromptVersion: DefaultPromptVersion},
		items: []Item{{ID: 1, JobID: "job", PhotoID: "photo", Path: "/library/photo.jpg"}},
	}
	worker := NewWorker(repository, fakeAnalyzer{result: json.RawMessage(`{"caption":"海邊"}`)})
	if err := worker.runJob(context.Background(), repository.job); err != nil {
		t.Fatal(err)
	}
	if string(repository.completed) != `{"caption":"海邊"}` {
		t.Fatalf("result = %s", repository.completed)
	}
}

func TestWorkerReturnsAnalyzerErrorToRepository(t *testing.T) {
	wanted := errors.New("offline")
	repository := &fakeRepository{
		job:   Job{ID: "job"},
		items: []Item{{ID: 1, JobID: "job", Path: "/photo.jpg"}},
	}
	worker := NewWorker(repository, fakeAnalyzer{err: wanted})
	if err := worker.runJob(context.Background(), repository.job); !errors.Is(err, wanted) {
		t.Fatalf("error = %v", err)
	}
}
