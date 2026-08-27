package batch

import (
	"context"
	"errors"
	"strings"
)

type Service struct {
	repository Repository
	importer   Importer
}

func NewService(repository Repository, importer Importer) *Service {
	return &Service{repository: repository, importer: importer}
}

func (s *Service) Create(ctx context.Context, input CreateInput) (Job, error) {
	if s.importer == nil {
		return Job{}, errors.New("batch importer is unavailable while the managed library is offline")
	}
	root, err := s.importer.ValidateBatchRoot(input.SourceRoot)
	if err != nil {
		return Job{}, err
	}
	input.SourceRoot = root
	input.Project = strings.TrimSpace(input.Project)
	if len(input.Tags) > 20 {
		return Job{}, errors.New("a batch job supports at most 20 shared tags")
	}
	if len(input.MediaTypes) == 0 {
		input.MediaTypes = []string{"photo", "video", "audio"}
	}
	seen := map[string]bool{}
	mediaTypes := make([]string, 0, len(input.MediaTypes))
	for _, mediaType := range input.MediaTypes {
		mediaType = strings.TrimSpace(strings.ToLower(mediaType))
		if mediaType != "photo" && mediaType != "video" && mediaType != "audio" {
			return Job{}, errors.New("mediaTypes supports only photo, video, and audio")
		}
		if !seen[mediaType] {
			seen[mediaType] = true
			mediaTypes = append(mediaTypes, mediaType)
		}
	}
	input.MediaTypes = mediaTypes
	return s.repository.Create(ctx, input)
}

func (s *Service) Get(ctx context.Context, id string) (Job, error) { return s.repository.Get(ctx, id) }
func (s *Service) Items(ctx context.Context, id string, limit int) ([]Item, error) {
	return s.repository.Items(ctx, id, limit)
}
func (s *Service) Cancel(ctx context.Context, id string) error { return s.repository.Cancel(ctx, id) }

func (s *Service) Resume(ctx context.Context, id string) (Job, error) {
	resumer, ok := s.repository.(Resumer)
	if !ok {
		return Job{}, errors.New("batch repository does not support resume")
	}
	return resumer.Resume(ctx, id)
}

func (s *Service) List(ctx context.Context, status string, limit int) ([]Job, error) {
	lister, ok := s.repository.(Lister)
	if !ok {
		return nil, errors.New("batch repository does not support listing jobs")
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status != "" && status != "pending" && status != "scanning" && status != "running" && status != "completed" && status != "completed_with_errors" && status != "failed" && status != "cancelled" {
		return nil, errors.New("invalid batch status")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return lister.List(ctx, status, limit)
}
