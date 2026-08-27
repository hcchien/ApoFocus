package batch

import (
	"context"
	"strings"
	"testing"
)

func TestCreateReturnsMaintenanceModeErrorWithoutImporter(t *testing.T) {
	service := NewService(&workerRepo{}, nil)
	_, err := service.Create(context.Background(), CreateInput{SourceRoot: "/Volumes/example"})
	if err == nil || !strings.Contains(err.Error(), "managed library is offline") {
		t.Fatalf("unexpected error: %v", err)
	}
}
