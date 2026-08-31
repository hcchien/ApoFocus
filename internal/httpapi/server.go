package httpapi

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
	"github.com/hcchien/apofocus/internal/initjob"
	webassets "github.com/hcchien/apofocus/web"
)

type Server struct {
	store     catalog.Store
	logger    *slog.Logger
	router    *http.ServeMux
	mediaRoot string
	folders   folders.Repository
	batchJobs *batch.Service
	media     catalog.MediaStore
	relations catalog.RelationStore
	initJobs  *initjob.Service
}

type Options struct {
	MediaRoot string
	Folders   folders.Repository
	BatchJobs *batch.Service
	Media     catalog.MediaStore
	Relations catalog.RelationStore
	InitJobs  *initjob.Service
}

func New(store catalog.Store, logger *slog.Logger) http.Handler {
	return NewWithMedia(store, logger, "")
}

func NewWithMedia(store catalog.Store, logger *slog.Logger, mediaRoot string) http.Handler {
	return NewWithOptions(store, logger, Options{MediaRoot: mediaRoot})
}

func NewWithOptions(store catalog.Store, logger *slog.Logger, options Options) http.Handler {
	if options.Relations == nil {
		options.Relations, _ = store.(catalog.RelationStore)
	}
	server := &Server{store: store, logger: logger, router: http.NewServeMux(), mediaRoot: options.MediaRoot, folders: options.Folders, batchJobs: options.BatchJobs, media: options.Media, relations: options.Relations, initJobs: options.InitJobs}
	server.routes()
	return server.recoverer(server.accessLog(server.securityHeaders(server.router)))
}

func (s *Server) routes() {
	s.router.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	s.router.HandleFunc("GET /api/v1/photos", s.listPhotos)
	s.router.HandleFunc("GET /api/v1/photos/{id}", s.getPhoto)
	s.router.HandleFunc("GET /api/v1/photos/{id}/file", s.servePhotoFile)
	s.router.HandleFunc("PATCH /api/v1/photos/{id}", s.updatePhoto)
	s.router.HandleFunc("GET /api/v1/photos/{id}/similar", s.similarPhotos)
	s.router.HandleFunc("GET /api/v1/facets", s.getFacets)
	if s.relations != nil {
		s.router.HandleFunc("GET /api/v1/relations/catalog", s.listRelationCatalog)
		s.router.HandleFunc("POST /api/v1/relations/bulk", s.bulkUpdateRelations)
		s.router.HandleFunc("POST /api/v1/projects", s.createProject)
		s.router.HandleFunc("PATCH /api/v1/projects/{id}", s.updateProjectDescription)
		s.router.HandleFunc("PUT /api/v1/projects/{id}/stories", s.replaceProjectStories)
		s.router.HandleFunc("POST /api/v1/stories", s.createStory)
		s.router.HandleFunc("PATCH /api/v1/stories/{id}", s.updateStoryDescription)
	}
	s.router.HandleFunc("GET /api/v1/videos", func(w http.ResponseWriter, r *http.Request) { s.listMedia(w, r, "video") })
	s.router.HandleFunc("GET /api/v1/videos/facets", func(w http.ResponseWriter, r *http.Request) { s.getMediaFacets(w, r, "video") })
	s.router.HandleFunc("GET /api/v1/videos/{id}", func(w http.ResponseWriter, r *http.Request) { s.getMedia(w, r, "video") })
	s.router.HandleFunc("GET /api/v1/videos/{id}/file", func(w http.ResponseWriter, r *http.Request) { s.serveCatalogMediaFile(w, r, "video") })
	s.router.HandleFunc("PATCH /api/v1/videos/{id}", func(w http.ResponseWriter, r *http.Request) { s.updateMedia(w, r, "video") })
	s.router.HandleFunc("GET /api/v1/videos/{id}/similar", func(w http.ResponseWriter, r *http.Request) { s.similarMedia(w, r, "video") })
	s.router.HandleFunc("GET /api/v1/audios", func(w http.ResponseWriter, r *http.Request) { s.listMedia(w, r, "audio") })
	s.router.HandleFunc("GET /api/v1/audios/facets", func(w http.ResponseWriter, r *http.Request) { s.getMediaFacets(w, r, "audio") })
	s.router.HandleFunc("GET /api/v1/audios/{id}", func(w http.ResponseWriter, r *http.Request) { s.getMedia(w, r, "audio") })
	s.router.HandleFunc("GET /api/v1/audios/{id}/file", func(w http.ResponseWriter, r *http.Request) { s.serveCatalogMediaFile(w, r, "audio") })
	s.router.HandleFunc("PATCH /api/v1/audios/{id}", func(w http.ResponseWriter, r *http.Request) { s.updateMedia(w, r, "audio") })
	s.router.HandleFunc("GET /api/v1/audios/{id}/similar", func(w http.ResponseWriter, r *http.Request) { s.similarMedia(w, r, "audio") })
	if s.mediaRoot != "" {
		s.router.HandleFunc("GET /media/{path...}", s.serveMedia)
	}
	s.router.HandleFunc("GET /api/v1/folders", s.getFolderTree)
	if s.folders != nil {
		s.router.HandleFunc("POST /api/v1/collections", s.createCollection)
		s.router.HandleFunc("POST /api/v1/collections/{id}/photos", s.addCollectionPhotos)
		s.router.HandleFunc("GET /api/v1/collections/{id}/photos", s.getCollectionPhotos)
	}
	if s.batchJobs != nil {
		s.router.HandleFunc("POST /api/v1/batch-jobs", s.createBatchJob)
		s.router.HandleFunc("GET /api/v1/batch-jobs/{id}", s.getBatchJob)
		s.router.HandleFunc("GET /api/v1/batch-jobs/{id}/items", s.getBatchItems)
		s.router.HandleFunc("GET /api/v1/batch-jobs/{id}/events", s.streamBatchEvents)
		s.router.HandleFunc("POST /api/v1/batch-jobs/{id}/cancel", s.cancelBatchJob)
	}
	if s.initJobs != nil {
		s.router.HandleFunc("POST /api/v1/init-runs", s.createInitRun)
		s.router.HandleFunc("GET /api/v1/init-runs", s.listInitRuns)
		s.router.HandleFunc("GET /api/v1/init-runs/{id}", s.getInitRun)
		s.router.HandleFunc("GET /api/v1/init-runs/{id}/items", s.getInitItems)
		s.router.HandleFunc("POST /api/v1/init-runs/{id}/pause", s.pauseInitRun)
		s.router.HandleFunc("POST /api/v1/init-runs/{id}/resume", s.resumeInitRun)
		s.router.HandleFunc("POST /api/v1/init-runs/{id}/cancel", s.cancelInitRun)
	}

	static, err := fs.Sub(webassets.Files, "static")
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(static))
	s.router.Handle("GET /", spaHandler(fileServer, static))
}

func (s *Server) serveMedia(w http.ResponseWriter, r *http.Request) {
	root, err := filepath.Abs(s.mediaRoot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "media root unavailable")
		return
	}
	target := filepath.Join(root, filepath.FromSlash(r.PathValue("path")))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		writeError(w, http.StatusForbidden, "invalid media path")
		return
	}
	stat, err := os.Stat(target)
	if err != nil || !stat.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	if strings.EqualFold(filepath.Ext(target), ".avif") {
		w.Header().Set("Content-Type", "image/avif")
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, target)
}

func (s *Server) servePhotoFile(w http.ResponseWriter, r *http.Request) {
	photo, err := s.store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	serveCatalogFile(w, r, photo.Path)
}
func (s *Server) serveCatalogMediaFile(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	asset, err := s.media.GetMedia(r.Context(), mediaType, r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "media not found")
		return
	}
	serveCatalogFile(w, r, asset.Path)
}
func serveCatalogFile(w http.ResponseWriter, r *http.Request, path string) {
	stat, err := os.Stat(path)
	if err != nil || !stat.Mode().IsRegular() {
		writeError(w, http.StatusNotFound, "media file unavailable")
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeFile(w, r, path)
}

func (s *Server) listPhotos(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := catalog.Filter{
		Query: query.Get("q"), Year: catalog.ParseInt(query.Get("year")), Project: query.Get("project"),
		Tags: nonEmpty(query["tag"]), Camera: query.Get("camera"), Lens: query.Get("lens"),
		MinISO: catalog.ParseInt(query.Get("min_iso")), MaxISO: catalog.ParseInt(query.Get("max_iso")),
		Limit: boundedInt(query.Get("limit"), 48, 1, 100), Offset: boundedInt(query.Get("offset"), 0, 0, 100000),
	}
	if value := query.Get("has_location"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "has_location must be true or false")
			return
		}
		filter.HasLocation = &parsed
	}
	page, err := s.store.List(r.Context(), filter)
	if err != nil {
		s.internalError(w, "list photos", err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getPhoto(w http.ResponseWriter, r *http.Request) {
	photo, err := s.store.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	if err != nil {
		s.internalError(w, "get photo", err)
		return
	}
	writeJSON(w, http.StatusOK, photo)
}

func (s *Server) similarPhotos(w http.ResponseWriter, r *http.Request) {
	limit := boundedInt(r.URL.Query().Get("limit"), 6, 1, 24)
	photos, err := s.store.Similar(r.Context(), r.PathValue("id"), limit)
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, "photo not found")
		return
	}
	if err != nil {
		s.internalError(w, "find similar photos", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": photos})
}

func (s *Server) getFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := s.store.Facets(r.Context())
	if err != nil {
		s.internalError(w, "get facets", err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (s *Server) internalError(w http.ResponseWriter, message string, err error) {
	s.logger.Error(message, "error", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func (s *Server) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/assets/") {
			s.logger.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
		}
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("panic", "value", recovered)
				writeError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func spaHandler(files http.Handler, static fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path != "" {
			if _, err := fs.Stat(static, path); err == nil {
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(static, "index.html")
		if err != nil {
			http.Error(w, "index unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(index)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func boundedInt(value string, fallback, minimum, maximum int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	if parsed < minimum {
		return minimum
	}
	if parsed > maximum {
		return maximum
	}
	return parsed
}

func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result = append(result, value)
		}
	}
	return result
}
