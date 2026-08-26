package catalog

import (
	"context"
	"testing"
)

func TestMemoryStoreListCombinesFilters(t *testing.T) {
	store := NewMemoryStore()
	hasLocation := true
	page, err := store.List(context.Background(), Filter{
		Project: "潮線", Tags: []string{"海岸"}, HasLocation: &hasLocation, Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 {
		t.Fatalf("expected 1 photo, got %d", page.Total)
	}
	if page.Items[0].ID != "a2" {
		t.Fatalf("expected a2, got %s", page.Items[0].ID)
	}
}

func TestMemoryStoreListSearchesMetadata(t *testing.T) {
	store := NewMemoryStore()
	page, err := store.List(context.Background(), Filter{Query: "Hasselblad", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("expected 2 photos, got %d", page.Total)
	}
}

func TestMemoryStoreSimilarExcludesAnchorAndSorts(t *testing.T) {
	store := NewMemoryStore()
	items, err := store.Similar(context.Background(), "a2", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 photos, got %d", len(items))
	}
	for i, item := range items {
		if item.Photo.ID == "a2" {
			t.Fatal("anchor photo must be excluded")
		}
		if i > 0 && item.Similarity > items[i-1].Similarity {
			t.Fatal("results are not sorted by similarity")
		}
	}
}

func TestMemoryStoreMissingPhoto(t *testing.T) {
	store := NewMemoryStore()
	if _, err := store.Get(context.Background(), "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
