package initjob

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var ErrNotFound = errors.New("init run not found")

type Service struct {
	repository  Repository
	importRoots []string
}

func NewService(repository Repository, roots []string) *Service {
	return &Service{repository: repository, importRoots: roots}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Run, error) {
	root, err := s.validateRoot(input.SourceRoot)
	if err != nil {
		return Run{}, err
	}
	input.SourceRoot = root
	input.Project = strings.TrimSpace(input.Project)
	input.Tags = cleanTags(input.Tags)
	return s.repository.Create(ctx, input)
}
func (s *Service) Get(ctx context.Context, id string) (Run, error) { return s.repository.Get(ctx, id) }
func (s *Service) List(ctx context.Context, status string, limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repository.List(ctx, status, limit)
}
func (s *Service) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	return s.repository.Items(ctx, id, limit)
}
func (s *Service) Pause(ctx context.Context, id string) error {
	return s.repository.RequestPause(ctx, id)
}
func (s *Service) Cancel(ctx context.Context, id string) error { return s.repository.Cancel(ctx, id) }
func (s *Service) Resume(ctx context.Context, id string) (Run, error) {
	return s.repository.Resume(ctx, id)
}
func (s *Service) validateRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("source root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("source root must be an existing directory")
	}
	for _, allowed := range s.importRoots {
		allowedAbs, _ := filepath.Abs(allowed)
		allowedResolved, e := filepath.EvalSymlinks(allowedAbs)
		if e != nil {
			continue
		}
		relative, e := filepath.Rel(allowedResolved, resolved)
		if e == nil && (relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
			return resolved, nil
		}
	}
	return "", errors.New("source root is outside APOFOCUS_IMPORT_ROOTS")
}
func cleanTags(values []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		key := strings.ToLower(v)
		if v != "" && !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}
