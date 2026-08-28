package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hcchien/apofocus/internal/catalog"
)

func testServer() http.Handler {
	return New(catalog.NewMemoryStore(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestListPhotosAPI(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/photos?year=2026&tag=海岸", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var page catalog.PhotoPage
	if err := json.NewDecoder(response.Body).Decode(&page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || page.Items[0].ID != "a2" {
		t.Fatalf("unexpected page: %+v", page)
	}
}

func TestSimilarPhotosAPI(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/photos/a2/similar?limit=3", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body struct {
		Items []catalog.SimilarPhoto `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(body.Items))
	}
}

func TestInvalidLocationFilter(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/photos?has_location=maybe", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", response.Code)
	}
}

func TestServesEmbeddedApp(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), "ApoFocus") {
		t.Fatal("embedded app title was not served")
	}
	for _, marker := range []string{`role="tablist"`, `data-media="photos"`, `data-media="videos"`, `data-media="audios"`, `class="audio-artwork detail-audio-artwork"`} {
		if !strings.Contains(response.Body.String(), marker) {
			t.Fatalf("embedded app is missing media tab marker %q", marker)
		}
	}
	if strings.Contains(response.Body.String(), `id="detail-audio-image"`) {
		t.Fatal("audio detail should not depend on a thumbnail image")
	}
}

func TestServesAVIFWithExplicitContentType(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "thumbnail.avif"), []byte("avif"), 0o640); err != nil {
		t.Fatal(err)
	}
	server := NewWithMedia(catalog.NewMemoryStore(), slog.New(slog.NewTextHandler(io.Discard, nil)), root)
	request := httptest.NewRequest(http.MethodGet, "/media/thumbnail.avif", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "image/avif" {
		t.Fatalf("unexpected Content-Type: %q", contentType)
	}
}

func TestFolderTreeIncludesVirtualSources(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/folders", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var body struct {
		Sources []struct {
			ID       string `json:"id"`
			Children []struct {
				Filter map[string]any `json:"filter"`
			} `json:"children"`
		} `json:"sources"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sources) != 5 || body.Sources[0].ID != "years" {
		t.Fatalf("unexpected folder sources: %+v", body.Sources)
	}
	if len(body.Sources[0].Children) == 0 {
		t.Fatal("year folders were not generated")
	}
}

func TestMediaFolderTreeWithoutDatabaseIsEmpty(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/folders?media=videos", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var body struct {
		Sources []any `json:"sources"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Sources) != 0 {
		t.Fatalf("expected no media folders without a media store, got %+v", body.Sources)
	}
}

func TestFolderTreeRejectsUnknownMedia(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api/v1/folders?media=documents", nil)
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
	}
}
