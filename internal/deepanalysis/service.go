package deepanalysis

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repository    Repository
	model         string
	promptVersion string
}

func NewService(repository Repository, model, promptVersion string) *Service {
	if strings.TrimSpace(model) == "" {
		model = DefaultModel
	}
	if strings.TrimSpace(promptVersion) == "" {
		promptVersion = DefaultPromptVersion
	}
	return &Service{repository: repository, model: model, promptVersion: promptVersion}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Job, error) {
	if len(input.PhotoIDs) == 0 || len(input.PhotoIDs) > 500 {
		return Job{}, errors.New("photoIds must contain between 1 and 500 IDs")
	}
	seen := make(map[string]bool, len(input.PhotoIDs))
	ids := make([]string, 0, len(input.PhotoIDs))
	for _, id := range input.PhotoIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			return Job{}, errors.New("photoIds must not contain empty IDs")
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	input.PhotoIDs = ids
	return s.repository.Create(ctx, input, s.model, s.promptVersion)
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) {
	return s.repository.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	if limit < 1 || limit > 500 {
		limit = 200
	}
	return s.repository.Items(ctx, strings.TrimSpace(id), limit)
}

func (s *Service) LatestForPhoto(ctx context.Context, id string) (PhotoAnalysis, error) {
	return s.repository.LatestForPhoto(ctx, strings.TrimSpace(id))
}

func (s *Service) Cancel(ctx context.Context, id string) error {
	return s.repository.Cancel(ctx, strings.TrimSpace(id))
}
