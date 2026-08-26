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
