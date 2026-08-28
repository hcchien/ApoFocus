package initjob

import (
	"path/filepath"
	"testing"
)

func TestCatalogStorageRootSplitsVolumesScan(t *testing.T) {
	if got := catalogStorageRoot("/Volumes", "/Volumes/Archive One/2024/photo.raw"); got != "/Volumes/Archive One" {
		t.Fatalf("expected per-volume root, got %q", got)
	}
	root := t.TempDir()
	if got := catalogStorageRoot(root, filepath.Join(root, "folder", "photo.jpg")); got != root {
		t.Fatalf("expected selected folder root, got %q", got)
	}
}

func TestCatalogProcessorExcludesManagedLibrary(t *testing.T) {
	library := filepath.Join(t.TempDir(), "ApoFocus Library")
	processor := NewCatalogProcessor(nil, library, nil, nil)
	if !processor.ExcludePath(filepath.Join(library, "thumbnails", "photo.avif")) {
		t.Fatal("expected managed output to be excluded")
	}
	if processor.ExcludePath(filepath.Join(filepath.Dir(library), "Archive", "photo.jpg")) {
		t.Fatal("expected archive outside the library to remain discoverable")
	}
}
