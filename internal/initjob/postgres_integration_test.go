package initjob

import (
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/ingest"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type integrationPhotoAnalyzer struct{}

func (integrationPhotoAnalyzer) Analyze(_ context.Context, _ string, thumbnail string) (ingest.Analysis, error) {
	if err := os.MkdirAll(filepath.Dir(thumbnail), 0o750); err != nil {
		return ingest.Analysis{}, err
	}
	if err := os.WriteFile(thumbnail, []byte("avif-test"), 0o640); err != nil {
		return ingest.Analysis{}, err
	}
	return ingest.Analysis{Embedding: make([]float32, 512), Tags: []string{"AI tag"}, DominantColor: "#112233"}, nil
}

func TestReferenceCatalogAndManualTagsBeatAIIntegration(t *testing.T) {
	databaseURL := os.Getenv("APOFOCUS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("APOFOCUS_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sourceRoot, libraryRoot := t.TempDir(), t.TempDir()
	source := filepath.Join(sourceRoot, "sample.jpg")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	pixels := image.NewRGBA(image.Rect(0, 0, 16, 16))
	draw.Draw(pixels, pixels.Bounds(), image.NewUniform(color.RGBA{R: 120, G: 80, B: 40, A: 255}), image.Point{}, draw.Src)
	if err = jpeg.Encode(file, pixels, nil); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(source)
	processor := NewCatalogProcessor(db, libraryRoot, integrationPhotoAnalyzer{}, nil)
	run := Run{ID: "00000000-0000-0000-0000-000000000001", SourceRoot: sourceRoot, Project: "Integration", Tags: []string{"shared"}}
	item := Item{ID: 1, SourcePath: source, MediaType: "photo", SizeBytes: info.Size(), ModifiedAt: timePointer(info.ModTime())}
	assetID, err := processor.Catalog(context.Background(), run, item)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM photos WHERE id=$1`, assetID)
		_, _ = db.Exec(`DELETE FROM storage_roots WHERE base_path IN ($1,$2)`, sourceRoot, libraryRoot)
	})
	store := catalog.NewPostgresStore(db)
	photo, err := store.Get(context.Background(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	manual := []string{"人工", "精選"}
	photo, err = store.Update(context.Background(), assetID, catalog.PhotoUpdate{Tags: &manual, Revision: &photo.Revision})
	if err != nil {
		t.Fatal(err)
	}
	item.AssetID = assetID
	if err = processor.AnalyzePhoto(context.Background(), run, item); err != nil {
		t.Fatal(err)
	}
	photo, err = store.Get(context.Background(), assetID)
	if err != nil {
		t.Fatal(err)
	}
	if len(photo.Tags) != 2 || photo.Tags[0] != "人工" || photo.AIStatus != "completed" || photo.HashStatus != "completed" {
		t.Fatalf("manual tags or AI status were not preserved: %+v", photo)
	}
}

func TestMediaMetadataEditIntegration(t *testing.T) {
	databaseURL := os.Getenv("APOFOCUS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("APOFOCUS_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var assetID string
	err = db.QueryRow(`INSERT INTO media_assets(media_type,title,capture_year,recorded_at,path,content_sha256,media_url)
		VALUES('audio','Original',2026,now(),$1,NULL,'/api/v1/audios/test/file') RETURNING id::text`, filepath.Join(t.TempDir(), "interview.wav")).Scan(&assetID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM media_assets WHERE id=$1`, assetID) })

	store := catalog.NewPostgresStore(db)
	asset, err := store.GetMedia(context.Background(), "audio", assetID)
	if err != nil {
		t.Fatal(err)
	}
	title, transcript := "Photographer title", "Corrected transcript"
	tags := []string{"interview", "manual"}
	asset, err = store.UpdateMedia(context.Background(), "audio", assetID, catalog.MediaUpdate{Title: &title, Tags: &tags, Transcript: &transcript, Revision: &asset.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if asset.Title != title || asset.Transcript != transcript || len(asset.Tags) != 2 {
		t.Fatalf("unexpected media edit: %+v", asset)
	}
	var tagsEdited, transcriptEdited bool
	if err = db.QueryRow(`SELECT tags_user_edited,transcript_user_edited FROM media_assets WHERE id=$1`, assetID).Scan(&tagsEdited, &transcriptEdited); err != nil {
		t.Fatal(err)
	}
	if !tagsEdited || !transcriptEdited {
		t.Fatalf("manual provenance flags were not set: tags=%v transcript=%v", tagsEdited, transcriptEdited)
	}
}
func timePointer(value time.Time) *time.Time { return &value }
