package mcpserver

import (
	"testing"

	"github.com/hcchien/apofocus/internal/initjob"
)

func TestInitStatusUsesPerMediaPhaseTotals(t *testing.T) {
	photo := initStatus(initjob.Run{Status: "photo_ai", DiscoveredCount: 100, PhotoCount: 20, MediaCount: 80, CatalogedCount: 100, PhotoAICount: 10})
	if photo.PhaseProgress != 50 || photo.OverallProgress != 52.5 {
		t.Fatalf("unexpected photo progress: %+v", photo)
	}
	media := initStatus(initjob.Run{Status: "media_ai", DiscoveredCount: 100, PhotoCount: 20, MediaCount: 80, CatalogedCount: 100, PhotoAICount: 20, MediaAICount: 40})
	if media.PhaseProgress != 50 || media.OverallProgress != 85 {
		t.Fatalf("unexpected media progress: %+v", media)
	}
}
