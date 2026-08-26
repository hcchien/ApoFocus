package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/hcchien/apofocus/internal/catalog"
)

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeJSON(w, http.StatusOK, catalog.MediaPage{Items: []catalog.MediaAsset{}, Limit: 60})
		return
	}
	query := r.URL.Query()
	filter := catalog.MediaFilter{
		MediaType:   mediaType,
		Query:       query.Get("q"),
		Year:        catalog.ParseInt(query.Get("year")),
		Project:     query.Get("project"),
		Tags:        nonEmpty(query["tag"]),
		Codec:       query.Get("codec"),
		MinDuration: int64(catalog.ParseInt(query.Get("min_duration_ms"))),
		MaxDuration: int64(catalog.ParseInt(query.Get("max_duration_ms"))),
		Limit:       boundedInt(query.Get("limit"), 60, 1, 100),
		Offset:      boundedInt(query.Get("offset"), 0, 0, 100000),
	}
	if value := query.Get("has_transcript"); value != "" {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			writeError(w, http.StatusBadRequest, "has_transcript must be true or false")
			return
		}
		filter.HasTranscript = &parsed
	}
	page, err := s.media.ListMedia(r.Context(), filter)
	if err != nil {
		s.internalError(w, "list "+mediaType, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) getMedia(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeError(w, http.StatusNotFound, mediaType+" not found")
		return
	}
	asset, err := s.media.GetMedia(r.Context(), mediaType, r.PathValue("id"))
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, mediaType+" not found")
		return
	}
	if err != nil {
		s.internalError(w, "get "+mediaType, err)
		return
	}
	writeJSON(w, http.StatusOK, asset)
}

func (s *Server) getMediaFacets(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeJSON(w, http.StatusOK, catalog.MediaFacets{Years: []catalog.FacetCount{}, Projects: []catalog.FacetCount{}, Tags: []catalog.FacetCount{}, Codecs: []catalog.FacetCount{}})
		return
	}
	facets, err := s.media.MediaFacets(r.Context(), mediaType)
	if err != nil {
		s.internalError(w, "get "+mediaType+" facets", err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func (s *Server) similarMedia(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []catalog.SimilarMedia{}})
		return
	}
	modality := r.URL.Query().Get("modality")
	if modality == "" {
		if mediaType == "video" {
			modality = "visual"
		} else {
			modality = "audio"
		}
	}
	if modality != "visual" && modality != "audio" {
		writeError(w, http.StatusBadRequest, "modality must be visual or audio")
		return
	}
	items, err := s.media.SimilarMedia(r.Context(), mediaType, r.PathValue("id"), modality, boundedInt(r.URL.Query().Get("limit"), 6, 1, 24))
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, mediaType+" not found")
		return
	}
	if err != nil {
		s.internalError(w, "find similar "+mediaType, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
