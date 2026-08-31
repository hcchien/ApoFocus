package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/hcchien/apofocus/internal/catalog"
)

const maxEditBody = 1 << 20

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-8][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}$`)

func (s *Server) updatePhoto(w http.ResponseWriter, r *http.Request) {
	var input catalog.PhotoUpdate
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePhotoUpdate(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	photo, err := s.store.Update(r.Context(), r.PathValue("id"), input)
	writeEditResult(w, photo, err, "photo")
}

func (s *Server) updateMedia(w http.ResponseWriter, r *http.Request, mediaType string) {
	if s.media == nil {
		writeError(w, http.StatusNotFound, mediaType+" not found")
		return
	}
	var input catalog.MediaUpdate
	if err := decodeEdit(r, &input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateMediaUpdate(input); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	asset, err := s.media.UpdateMedia(r.Context(), mediaType, r.PathValue("id"), input)
	writeEditResult(w, asset, err, mediaType)
}

func decodeEdit(r *http.Request, target any) error {
	decoder := json.NewDecoder(io.LimitReader(r.Body, maxEditBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid edit payload: " + err.Error())
	}
	return nil
}

func validatePhotoUpdate(input catalog.PhotoUpdate) error {
	if input.Revision == nil {
		return errors.New("revision is required")
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return errors.New("title cannot be empty")
	}
	if input.Rating != nil && (*input.Rating < 0 || *input.Rating > 5) {
		return errors.New("rating must be between 0 and 5")
	}
	if input.ISO != nil && *input.ISO < 0 {
		return errors.New("iso cannot be negative")
	}
	if input.Location != nil && (input.Location.Latitude < -90 || input.Location.Latitude > 90 || input.Location.Longitude < -180 || input.Location.Longitude > 180) {
		return errors.New("invalid location coordinates")
	}
	if input.Tags != nil && len(*input.Tags) > 100 {
		return errors.New("at most 100 tags are allowed")
	}
	if err := validateRelationIDs("projectIds", input.ProjectIDs); err != nil {
		return err
	}
	if err := validateRelationIDs("storyIds", input.StoryIDs); err != nil {
		return err
	}
	if err := validateDerivations("parents", input.Parents); err != nil {
		return err
	}
	if err := validateDerivations("children", input.Children); err != nil {
		return err
	}
	return nil
}

func validateMediaUpdate(input catalog.MediaUpdate) error {
	if input.Revision == nil {
		return errors.New("revision is required")
	}
	if input.Title != nil && strings.TrimSpace(*input.Title) == "" {
		return errors.New("title cannot be empty")
	}
	if input.Rating != nil && (*input.Rating < 0 || *input.Rating > 5) {
		return errors.New("rating must be between 0 and 5")
	}
	if input.Tags != nil && len(*input.Tags) > 100 {
		return errors.New("at most 100 tags are allowed")
	}
	if err := validateRelationIDs("projectIds", input.ProjectIDs); err != nil {
		return err
	}
	if err := validateRelationIDs("storyIds", input.StoryIDs); err != nil {
		return err
	}
	if err := validateRelationIDs("relatedAssetIds", input.RelatedAssetIDs); err != nil {
		return err
	}
	return nil
}

func validateRelationIDs(field string, values *[]string) error {
	if values == nil {
		return nil
	}
	if len(*values) > 500 {
		return errors.New(field + " allows at most 500 IDs")
	}
	for _, value := range *values {
		if !uuidPattern.MatchString(strings.TrimSpace(value)) {
			return errors.New(field + " must contain UUIDs")
		}
	}
	return nil
}

func validateDerivations(field string, values *[]catalog.PhotoDerivationInput) error {
	if values == nil {
		return nil
	}
	if len(*values) > 500 {
		return errors.New(field + " allows at most 500 relationships")
	}
	for _, value := range *values {
		if !uuidPattern.MatchString(strings.TrimSpace(value.PhotoID)) {
			return errors.New(field + " photoId must be a UUID")
		}
		if len([]rune(value.RelationType)) > 200 {
			return errors.New(field + " relationType is too long")
		}
	}
	return nil
}

func validateBulkRelationUpdate(input catalog.BulkRelationUpdate) error {
	if input.MediaType != "photo" && input.MediaType != "video" && input.MediaType != "audio" {
		return errors.New("mediaType must be photo, video, or audio")
	}
	if input.Operation != "add" && input.Operation != "remove" && input.Operation != "replace" {
		return errors.New("operation must be add, remove, or replace")
	}
	assetIDs := input.AssetIDs
	if len(assetIDs) == 0 {
		return errors.New("assetIds must not be empty")
	}
	if err := validateRelationIDs("assetIds", &assetIDs); err != nil {
		return err
	}
	if err := validateRelationIDs("projectIds", input.ProjectIDs); err != nil {
		return err
	}
	if err := validateRelationIDs("storyIds", input.StoryIDs); err != nil {
		return err
	}
	if input.ProjectIDs == nil && input.StoryIDs == nil && input.PhotoRelation == nil {
		return errors.New("at least one relationship change is required")
	}
	if input.PhotoRelation == nil {
		return nil
	}
	if input.MediaType != "photo" {
		return errors.New("photoRelation is only available for photos")
	}
	if input.Operation == "replace" {
		return errors.New("photoRelation supports only add or remove")
	}
	if input.PhotoRelation.Direction != "children_of" && input.PhotoRelation.Direction != "parents_of" {
		return errors.New("photoRelation.direction must be children_of or parents_of")
	}
	if !uuidPattern.MatchString(strings.TrimSpace(input.PhotoRelation.OtherPhotoID)) {
		return errors.New("photoRelation.otherPhotoId must be a UUID")
	}
	if len([]rune(input.PhotoRelation.RelationType)) > 200 {
		return errors.New("photoRelation.relationType is too long")
	}
	for _, assetID := range input.AssetIDs {
		if strings.EqualFold(strings.TrimSpace(assetID), strings.TrimSpace(input.PhotoRelation.OtherPhotoID)) {
			return errors.New("a photo cannot be related to itself")
		}
	}
	return nil
}

func writeEditResult[T any](w http.ResponseWriter, value T, err error, kind string) {
	if errors.Is(err, catalog.ErrNotFound) {
		writeError(w, http.StatusNotFound, kind+" not found")
		return
	}
	if errors.Is(err, catalog.ErrConflict) {
		writeError(w, http.StatusConflict, "record changed; reload before saving")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not update "+kind)
		return
	}
	writeJSON(w, http.StatusOK, value)
}
