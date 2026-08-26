package storagewatch

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/fileidentity"
)

func TestPostgresRepositoryTracksMovedPhoto(t *testing.T) {
	databaseURL := os.Getenv("APOFOCUS_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("APOFOCUS_TEST_DATABASE_URL is not set")
	}
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	rootPath := t.TempDir()
	originalPath := filepath.Join(rootPath, "original.jpg")
	if err := os.WriteFile(originalPath, []byte("photo"), 0o600); err != nil {
		t.Fatal(err)
	}
	contentHash := fmt.Sprintf("storagewatch-%d", time.Now().UnixNano())
	var photoID string
	if err := db.QueryRowContext(ctx, `INSERT INTO photos(title,capture_year,taken_at,path,content_sha256,image_url,thumbnail_url)
		VALUES('watcher integration',2026,now(),$1,$2,'/media/original.jpg','') RETURNING id::text`, originalPath, contentHash).Scan(&photoID); err != nil {
		t.Fatal(err)
	}
	var rootID string
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM photos WHERE id=$1`, photoID)
		if rootID != "" {
			_, _ = db.ExecContext(context.Background(), `DELETE FROM storage_roots WHERE id=$1`, rootID)
		}
	})

	repository := NewPostgresRepository(db)
	root, err := repository.EnsureRoot(ctx, rootPath)
	if err != nil {
		t.Fatal(err)
	}
	rootID = root.ID
	canonicalOriginalPath := filepath.Join(root.BasePath, "original.jpg")
	assertPhotoTracking(t, db, photoID, canonicalOriginalPath, "original.jpg", "/media/original.jpg", "available")
	assertCatalogAvailability(t, db, photoID, "available")

	movedPath := filepath.Join(root.BasePath, "selected", "moved.jpg")
	if err := os.MkdirAll(filepath.Dir(movedPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(canonicalOriginalPath, movedPath); err != nil {
		t.Fatal(err)
	}
	identity, err := fileidentity.FromPath(movedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ObservePath(ctx, root, movedPath, identity); err != nil {
		t.Fatal(err)
	}
	assertPhotoTracking(t, db, photoID, movedPath, "selected/moved.jpg", "/media/selected/moved.jpg", "available")
	assertCatalogAvailability(t, db, photoID, "available")

	if err := repository.MarkMissing(ctx, root, movedPath); err != nil {
		t.Fatal(err)
	}
	assertPhotoTracking(t, db, photoID, movedPath, "selected/moved.jpg", "/media/selected/moved.jpg", "missing")
	assertCatalogAvailability(t, db, photoID, "missing")

	if err := repository.MarkRootOffline(ctx, root); err != nil {
		t.Fatal(err)
	}
	assertCatalogAvailability(t, db, photoID, "volume_offline")
	assertRootStatus(t, db, root.ID, "offline")
	if err := repository.ObservePath(ctx, root, movedPath, identity); err != nil {
		t.Fatal(err)
	}
	assertCatalogAvailability(t, db, photoID, "available")
	assertRootStatus(t, db, root.ID, "online")
}

func assertRootStatus(t *testing.T, db *sql.DB, rootID, want string) {
	t.Helper()
	var status string
	if err := db.QueryRow(`SELECT status FROM storage_roots WHERE id=$1`, rootID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != want {
		t.Fatalf("storage root status = %q, want %q", status, want)
	}
}

func assertCatalogAvailability(t *testing.T, db *sql.DB, photoID, want string) {
	t.Helper()
	photo, err := catalog.NewPostgresStore(db).Get(context.Background(), photoID)
	if err != nil {
		t.Fatal(err)
	}
	if photo.Availability != want {
		t.Fatalf("catalog availability = %q, want %q", photo.Availability, want)
	}
}

func assertPhotoTracking(t *testing.T, db *sql.DB, photoID, wantPath, wantRelative, wantURL, wantStatus string) {
	t.Helper()
	var path, relative, fileID, imageURL, status string
	if err := db.QueryRow(`SELECT path,relative_path,file_id,image_url,availability_status FROM photos WHERE id=$1`, photoID).
		Scan(&path, &relative, &fileID, &imageURL, &status); err != nil {
		t.Fatal(err)
	}
	if path != wantPath || relative != wantRelative || imageURL != wantURL || status != wantStatus || fileID == "" {
		t.Fatalf("tracking mismatch: path=%q relative=%q fileID=%q url=%q status=%q", path, relative, fileID, imageURL, status)
	}
}
