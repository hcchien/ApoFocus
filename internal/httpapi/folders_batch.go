package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/hcchien/apofocus/internal/batch"
	"github.com/hcchien/apofocus/internal/catalog"
	"github.com/hcchien/apofocus/internal/folders"
)

type folderNode struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Kind     string         `json:"kind"`
	Count    int            `json:"count,omitempty"`
	Filter   map[string]any `json:"filter,omitempty"`
	Children []folderNode   `json:"children,omitempty"`
}

func (s *Server) getFolderTree(w http.ResponseWriter, r *http.Request) {
	requestedMedia := r.URL.Query().Get("media")
	if requestedMedia == "videos" || requestedMedia == "audios" {
		mediaType := requestedMedia[:len(requestedMedia)-1]
		if s.media == nil {
			writeJSON(w, http.StatusOK, map[string]any{"sources": []folderNode{}, "collections": []folders.Collection{}})
			return
		}
		facets, err := s.media.MediaFacets(r.Context(), mediaType)
		if err != nil {
			s.internalError(w, "get media folder facets", err)
			return
		}
		category := func(id, name, key string, items []catalog.FacetCount) folderNode {
			children := make([]folderNode, 0, len(items))
			for _, item := range items {
				var value any = item.Value
				if key == "year" {
					value = catalog.ParseInt(item.Value)
				}
				children = append(children, folderNode{ID: id + ":" + item.Value, Name: item.Value, Kind: "virtual", Count: item.Count, Filter: map[string]any{key: value}})
			}
			return folderNode{ID: id, Name: name, Kind: "category", Children: children}
		}
		yearLabel := "錄製年份"
		if requestedMedia == "videos" {
			yearLabel = "拍攝年份"
		}
		sources := []folderNode{
			category("years", yearLabel, "year", facets.Years),
			category("projects", "專題", "project", facets.Projects),
			category("tags", "Tags", "tag", facets.Tags),
			category("codecs", "Codec", "codec", facets.Codecs),
		}
		writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "collections": []folders.Collection{}})
		return
	}
	if requestedMedia != "" && requestedMedia != "photos" {
		writeError(w, http.StatusBadRequest, "media must be photos, videos, or audios")
		return
	}
	facets, err := s.store.Facets(r.Context())
	if err != nil {
		s.internalError(w, "get folder facets", err)
		return
	}
	category := func(id, name, key string, items []catalog.FacetCount) folderNode {
		children := make([]folderNode, 0, len(items))
		for _, item := range items {
			var value any = item.Value
			if key == "year" {
				value = catalog.ParseInt(item.Value)
			}
			children = append(children, folderNode{ID: id + ":" + item.Value, Name: item.Value, Kind: "virtual", Count: item.Count, Filter: map[string]any{key: value}})
		}
		return folderNode{ID: id, Name: name, Kind: "category", Children: children}
	}
	sources := []folderNode{category("years", "拍照年份", "year", facets.Years), category("projects", "專題", "project", facets.Projects), category("tags", "Tags", "tag", facets.Tags), category("cameras", "相機", "camera", facets.Cameras), category("lenses", "鏡頭", "lens", facets.Lenses)}
	collections := []folders.Collection{}
	if s.folders != nil {
		collections, err = s.folders.List(r.Context())
		if err != nil {
			s.internalError(w, "list collections", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"sources": sources, "collections": collections})
}

func (s *Server) createCollection(w http.ResponseWriter, r *http.Request) {
	var input folders.CreateInput
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := s.folders.Create(r.Context(), input)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) addCollectionPhotos(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PhotoIDs []string `json:"photoIds"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.folders.AddPhotos(r.Context(), r.PathValue("id"), input.PhotoIDs); errors.Is(err, folders.ErrNotFound) {
		writeError(w, http.StatusNotFound, "collection not found")
		return
	} else if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": len(input.PhotoIDs)})
}

func (s *Server) getCollectionPhotos(w http.ResponseWriter, r *http.Request) {
	ids, err := s.folders.PhotoIDs(r.Context(), r.PathValue("id"))
	if err != nil {
		s.internalError(w, "list collection photos", err)
		return
	}
	photos := make([]catalog.Photo, 0, len(ids))
	for _, id := range ids {
		photo, err := s.store.Get(r.Context(), id)
		if err == nil {
			photos = append(photos, photo)
		}
	}
	writeJSON(w, http.StatusOK, catalog.PhotoPage{Items: photos, Total: len(photos), Limit: len(photos)})
}

type createBatchRequest struct {
	SourceRoot string   `json:"sourceRoot"`
	Project    string   `json:"project"`
	Tags       []string `json:"tags"`
	Recursive  *bool    `json:"recursive"`
	AutoTags   *bool    `json:"autoTags"`
	MediaTypes []string `json:"mediaTypes"`
}

func (s *Server) createBatchJob(w http.ResponseWriter, r *http.Request) {
	var input createBatchRequest
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	recursive, auto := true, true
	if input.Recursive != nil {
		recursive = *input.Recursive
	}
	if input.AutoTags != nil {
		auto = *input.AutoTags
	}
	job, err := s.batchJobs.Create(r.Context(), batch.CreateInput{SourceRoot: input.SourceRoot, Project: input.Project, Tags: input.Tags, Recursive: recursive, AutoTags: auto, MediaTypes: input.MediaTypes})
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Location", "/api/v1/batch-jobs/"+job.ID)
	writeJSON(w, http.StatusAccepted, job)
}
func (s *Server) getBatchJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.batchJobs.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, batch.ErrNotFound) {
		writeError(w, http.StatusNotFound, "batch job not found")
		return
	} else if err != nil {
		s.internalError(w, "get batch job", err)
		return
	}
	writeJSON(w, http.StatusOK, job)
}
func (s *Server) getBatchItems(w http.ResponseWriter, r *http.Request) {
	limit := boundedInt(r.URL.Query().Get("limit"), 200, 1, 1000)
	items, err := s.batchJobs.Items(r.Context(), r.PathValue("id"), limit)
	if err != nil {
		s.internalError(w, "get batch items", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
func (s *Server) cancelBatchJob(w http.ResponseWriter, r *http.Request) {
	if err := s.batchJobs.Cancel(r.Context(), r.PathValue("id")); errors.Is(err, batch.ErrNotFound) {
		writeError(w, http.StatusNotFound, "batch job not found")
		return
	} else if err != nil {
		s.internalError(w, "cancel batch job", err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "cancellation_requested"})
}

func (s *Server) streamBatchEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()
	for {
		job, err := s.batchJobs.Get(r.Context(), r.PathValue("id"))
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", strconv.Quote(err.Error()))
			flusher.Flush()
			return
		}
		data, _ := json.Marshal(job)
		event := "progress"
		if job.Terminal() {
			event = "complete"
		}
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, data)
		flusher.Flush()
		if job.Terminal() {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func decodeJSON(r *http.Request, value any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}
