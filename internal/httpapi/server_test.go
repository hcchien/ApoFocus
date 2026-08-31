package httpapi

import (
	"context"
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

type fakeRelationStore struct{}

func (fakeRelationStore) ListRelationCatalog(context.Context) (catalog.RelationCatalog, error) {
	return catalog.RelationCatalog{Projects: []catalog.Project{{ID: "project-id", Description: "Project description"}}, Stories: []catalog.Story{{ID: "story-id", Description: "Story description"}}}, nil
}
func (fakeRelationStore) CreateProject(_ context.Context, description string) (catalog.Project, error) {
	return catalog.Project{ID: "project-id", Description: description}, nil
}
func (fakeRelationStore) UpdateProject(_ context.Context, id, description string) (catalog.Project, error) {
	return catalog.Project{ID: id, Description: description}, nil
}
func (fakeRelationStore) CreateStory(_ context.Context, description string) (catalog.Story, error) {
	return catalog.Story{ID: "story-id", Description: description}, nil
}
func (fakeRelationStore) UpdateStory(_ context.Context, id, description string) (catalog.Story, error) {
	return catalog.Story{ID: id, Description: description}, nil
}
func (fakeRelationStore) ReplaceProjectStories(_ context.Context, _ string, storyIDs []string) ([]catalog.Story, error) {
	items := make([]catalog.Story, len(storyIDs))
	for index, id := range storyIDs {
		items[index] = catalog.Story{ID: id, Description: "linked"}
	}
	return items, nil
}
func (fakeRelationStore) BulkUpdateRelations(_ context.Context, input catalog.BulkRelationUpdate) (catalog.BulkRelationResult, error) {
	return catalog.BulkRelationResult{MediaType: input.MediaType, Operation: input.Operation, AssetCount: len(input.AssetIDs)}, nil
}

func relationTestServer() http.Handler {
	return NewWithOptions(catalog.NewMemoryStore(), slog.New(slog.NewTextHandler(io.Discard, nil)), Options{Relations: fakeRelationStore{}})
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

func TestUpdatePhotoAPI(t *testing.T) {
	server := testServer()
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/photos/a1", strings.NewReader(`{"title":"人工標題","tags":["人工","精選"],"rating":5,"revision":0}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	var photo catalog.Photo
	if err := json.NewDecoder(response.Body).Decode(&photo); err != nil {
		t.Fatal(err)
	}
	if photo.Title != "人工標題" || photo.Rating != 5 || len(photo.Tags) != 2 {
		t.Fatalf("unexpected update: %+v", photo)
	}
}

func TestUpdatePhotoRequiresRevision(t *testing.T) {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/photos/a1", strings.NewReader(`{"title":"沒有 revision"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "revision is required") {
		t.Fatalf("expected revision validation, got %d: %s", response.Code, response.Body.String())
	}
}

func TestRelationCatalogAndEntityWrites(t *testing.T) {
	server := relationTestServer()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/relations/catalog", nil)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Project description") || !strings.Contains(response.Body.String(), "Story description") {
		t.Fatalf("unexpected relation catalog: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/stories", strings.NewReader(`{"description":"A new story"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), "A new story") {
		t.Fatalf("unexpected story create: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPatch, "/api/v1/projects/project-id", strings.NewReader(`{"description":"Updated project"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Updated project") {
		t.Fatalf("unexpected project update: %d %s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPut, "/api/v1/projects/00000000-0000-4000-8000-000000000001/stories", strings.NewReader(`{"storyIds":["00000000-0000-4000-8000-000000000002"]}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "00000000-0000-4000-8000-000000000002") {
		t.Fatalf("unexpected project-story update: %d %s", response.Code, response.Body.String())
	}
}

func TestPhotoRelationshipUpdateRejectsNonUUIDs(t *testing.T) {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/photos/a1", strings.NewReader(`{"projectIds":["not-a-uuid"],"revision":0}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	testServer().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "must contain UUIDs") {
		t.Fatalf("expected UUID validation, got %d: %s", response.Code, response.Body.String())
	}
}

func TestBulkRelationshipUpdate(t *testing.T) {
	server := relationTestServer()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/relations/bulk", strings.NewReader(`{
		"mediaType":"photo","assetIds":["00000000-0000-4000-8000-000000000001","00000000-0000-4000-8000-000000000002"],
		"operation":"add","projectIds":["00000000-0000-4000-8000-000000000003"],
		"photoRelation":{"direction":"children_of","otherPhotoId":"00000000-0000-4000-8000-000000000004","relationType":"raw_export"}
	}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"assetCount":2`) {
		t.Fatalf("unexpected bulk update: %d %s", response.Code, response.Body.String())
	}
}

func TestBulkRelationshipUpdateValidation(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty selection", `{"mediaType":"photo","assetIds":[],"operation":"add","projectIds":[]}`, "must not be empty"},
		{"invalid operation", `{"mediaType":"photo","assetIds":["00000000-0000-4000-8000-000000000001"],"operation":"merge","projectIds":[]}`, "operation must be"},
		{"self relation", `{"mediaType":"photo","assetIds":["00000000-0000-4000-8000-000000000001"],"operation":"add","photoRelation":{"direction":"children_of","otherPhotoId":"00000000-0000-4000-8000-000000000001"}}`, "cannot be related to itself"},
		{"media derivation", `{"mediaType":"video","assetIds":["00000000-0000-4000-8000-000000000001"],"operation":"add","photoRelation":{"direction":"children_of","otherPhotoId":"00000000-0000-4000-8000-000000000002"}}`, "only available for photos"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/relations/bulk", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			relationTestServer().ServeHTTP(response, request)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("expected validation %q, got %d %s", test.want, response.Code, response.Body.String())
			}
		})
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
	for _, marker := range []string{`role="tablist"`, `data-media="photos"`, `data-media="videos"`, `data-media="audios"`, `class="audio-artwork detail-audio-artwork"`, `id="detail-relations"`, `id="media-detail-relations"`, `data-relation-options="projects"`, `data-relation-options="stories"`, `id="bulk-relations-dialog"`, `id="select-visible"`} {
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
