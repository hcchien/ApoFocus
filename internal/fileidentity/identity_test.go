package fileidentity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIdentitySurvivesRename(t *testing.T) {
	dir := t.TempDir()
	before := filepath.Join(dir, "before.jpg")
	after := filepath.Join(dir, "after.jpg")
	if err := os.WriteFile(before, []byte("photo"), 0o640); err != nil {
		t.Fatal(err)
	}
	first, err := FromPath(before)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(before, after); err != nil {
		t.Fatal(err)
	}
	second, err := FromPath(after)
	if err != nil {
		t.Fatal(err)
	}
	if first.FileID == "" || first.FileID != second.FileID || first.VolumeID != second.VolumeID {
		t.Fatalf("identity changed across rename: before=%+v after=%+v", first, second)
	}
}
