package mediaingest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

const maxMediaBytes int64 = 64 << 30

var videoExtensions = map[string]bool{".mp4": true, ".mov": true, ".m4v": true, ".mkv": true, ".avi": true, ".webm": true, ".mts": true, ".m2ts": true}
var audioExtensions = map[string]bool{".wav": true, ".mp3": true, ".m4a": true, ".aac": true, ".flac": true, ".ogg": true, ".opus": true, ".aiff": true, ".aif": true}

func DetectMediaType(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	if videoExtensions[extension] {
		return "video"
	}
	if audioExtensions[extension] {
		return "audio"
	}
	return ""
}

type Manager struct {
	libraryRoot string
	importRoots []string
	analyzer    Analyzer
	repository  Repository
}

func NewManager(libraryRoot string, importRoots []string, analyzer Analyzer, repository Repository) (*Manager, error) {
	root, err := cleanRoot(libraryRoot)
	if err != nil {
		return nil, fmt.Errorf("library root: %w", err)
	}
	cleanImports := make([]string, 0, len(importRoots))
	for _, value := range importRoots {
		if strings.TrimSpace(value) == "" {
			continue
		}
		clean, err := cleanRoot(value)
		if err != nil {
			return nil, fmt.Errorf("import root %q: %w", value, err)
		}
		cleanImports = append(cleanImports, clean)
	}
	if len(cleanImports) == 0 {
		return nil, errors.New("at least one valid import root is required")
	}
	if analyzer == nil || repository == nil {
		return nil, errors.New("analyzer and repository are required")
	}
	return &Manager{libraryRoot: root, importRoots: cleanImports, analyzer: analyzer, repository: repository}, nil
}

func (m *Manager) Import(ctx context.Context, request ImportRequest) (ImportResult, error) {
	source, stat, err := m.resolveSource(request.SourcePath)
	if err != nil {
		return ImportResult{}, err
	}
	mediaType := DetectMediaType(source)
	if mediaType == "" {
		return ImportResult{}, errors.New("unsupported video or audio file type")
	}
	if stat.Size() > maxMediaBytes {
		return ImportResult{}, fmt.Errorf("media exceeds the %d byte limit", maxMediaBytes)
	}
	hash, err := hashFile(source)
	if err != nil {
		return ImportResult{}, err
	}
	if existing, found, err := m.repository.FindByHash(ctx, hash); err != nil {
		return ImportResult{}, err
	} else if found {
		return ImportResult{AssetID: existing.ID, MediaType: existing.MediaType, Path: existing.Path, ThumbnailPath: existing.ThumbnailPath, Tags: existing.Tags, AlreadyExists: true}, nil
	}

	stagingDir := filepath.Join(m.libraryRoot, ".staging", hash)
	if err := os.MkdirAll(stagingDir, 0o750); err != nil {
		return ImportResult{}, err
	}
	defer os.RemoveAll(stagingDir)
	stagingThumbnail := filepath.Join(stagingDir, "thumbnail.jpg")
	stagingSegments := filepath.Join(stagingDir, "segments")
	analysis, err := m.analyzer.AnalyzeMedia(ctx, source, stagingThumbnail, stagingSegments, request.AutoTags)
	if err != nil {
		return ImportResult{}, err
	}
	if analysis.MediaType != mediaType {
		return ImportResult{}, fmt.Errorf("extension suggests %s but analyzer detected %s", mediaType, analysis.MediaType)
	}
	recordedAt := stat.ModTime()
	if parsed, parseErr := time.Parse(time.RFC3339Nano, analysis.RecordedAt); parseErr == nil {
		recordedAt = parsed
	}
	project := cleanLabel(request.Project, 120)
	if project == "" {
		project = "未分類"
	}
	title := cleanLabel(request.Title, 240)
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(source), filepath.Ext(source))
	}
	originalPath, thumbnailPath, segmentDir := m.destinationPaths(mediaType, project, title, hash, filepath.Ext(source), recordedAt)
	if err := moveAnalyzedFiles(stagingThumbnail, stagingSegments, thumbnailPath, segmentDir, analysis.Segments); err != nil {
		return ImportResult{}, err
	}
	createdOriginal, err := copyFile(source, originalPath)
	if err != nil {
		_ = os.RemoveAll(segmentDir)
		_ = os.Remove(thumbnailPath)
		return ImportResult{}, err
	}
	cleanup := func() {
		if createdOriginal {
			_ = os.Remove(originalPath)
		}
		_ = os.Remove(thumbnailPath)
		_ = os.RemoveAll(segmentDir)
	}
	for index := range analysis.Segments {
		if analysis.Segments[index].KeyframePath != "" {
			analysis.Segments[index].KeyframeURL = mediaURL(m.libraryRoot, analysis.Segments[index].KeyframePath)
		}
	}
	record := Record{
		MediaType: mediaType, Title: title, Year: recordedAt.Year(), Project: project, RecordedAt: recordedAt,
		DurationMS: analysis.DurationMS, MimeType: analysis.MimeType, Codec: analysis.Codec, Dimensions: analysis.Dimensions,
		SampleRate: analysis.SampleRate, Channels: analysis.Channels, Path: originalPath, ThumbnailPath: thumbnailPath,
		ContentSHA256: hash, MediaURL: mediaURL(m.libraryRoot, originalPath), ThumbnailURL: mediaURL(m.libraryRoot, thumbnailPath),
		Transcript: analysis.Transcript, Tags: mergeTags(request.Tags, analysis.Tags), Metadata: analysis.Metadata, Segments: analysis.Segments,
	}
	assetID, err := m.repository.Insert(ctx, record)
	if err != nil {
		cleanup()
		return ImportResult{}, err
	}
	return ImportResult{AssetID: assetID, MediaType: mediaType, Path: originalPath, ThumbnailPath: thumbnailPath, Tags: record.Tags, SegmentCount: len(record.Segments), TranscriptLength: len([]rune(record.Transcript))}, nil
}

func (m *Manager) resolveSource(value string) (string, os.FileInfo, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil, errors.New("source path is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", nil, err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source path: %w", err)
	}
	allowed := false
	for _, root := range m.importRoots {
		relative, relErr := filepath.Rel(root, resolved)
		if relErr == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, errors.New("source path is outside APOFOCUS_IMPORT_ROOTS")
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", nil, err
	}
	if !stat.Mode().IsRegular() {
		return "", nil, errors.New("source path must be a regular file")
	}
	return resolved, stat, nil
}

func (m *Manager) destinationPaths(mediaType, project, title, hash, extension string, recordedAt time.Time) (string, string, string) {
	year := fmt.Sprintf("%04d", recordedAt.Year())
	kind := mediaType + "s"
	base := safeSegment(title)
	filename := fmt.Sprintf("%s_%s_%s%s", recordedAt.Format("2006-01-02"), base, hash[:10], strings.ToLower(extension))
	stem := strings.TrimSuffix(filename, filepath.Ext(filename))
	return filepath.Join(m.libraryRoot, "originals", kind, year, safeSegment(project), filename),
		filepath.Join(m.libraryRoot, "thumbnails", kind, year, safeSegment(project), stem+".jpg"),
		filepath.Join(m.libraryRoot, "segments", kind, year, safeSegment(project), stem)
}

func moveAnalyzedFiles(stagingThumbnail, stagingSegments, thumbnailPath, segmentDir string, segments []Segment) error {
	thumbnailMoved := false
	segmentsMoved := false
	cleanup := func() {
		if segmentsMoved {
			_ = os.RemoveAll(segmentDir)
		}
		if thumbnailMoved {
			_ = os.Remove(thumbnailPath)
		}
	}
	if err := os.MkdirAll(filepath.Dir(thumbnailPath), 0o750); err != nil {
		return err
	}
	if err := os.Rename(stagingThumbnail, thumbnailPath); err != nil {
		return fmt.Errorf("move thumbnail: %w", err)
	}
	thumbnailMoved = true
	if _, err := os.Stat(stagingSegments); err == nil {
		if err := os.MkdirAll(filepath.Dir(segmentDir), 0o750); err != nil {
			cleanup()
			return err
		}
		if err := os.Rename(stagingSegments, segmentDir); err != nil {
			cleanup()
			return fmt.Errorf("move media segments: %w", err)
		}
		segmentsMoved = true
	}
	for index := range segments {
		if segments[index].KeyframePath != "" {
			segments[index].KeyframePath = filepath.Join(segmentDir, filepath.Base(segments[index].KeyframePath))
		}
	}
	return nil
}

func cleanRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("path is empty")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(absolute)
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyFile(source, destination string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return false, err
	}
	input, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errors.Is(err, os.ErrExist) {
		existingHash, hashErr := hashFile(destination)
		sourceHash, sourceErr := hashFile(source)
		if hashErr == nil && sourceErr == nil && existingHash == sourceHash {
			return false, nil
		}
		return false, errors.New("destination exists with different content")
	}
	if err != nil {
		return false, err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		_ = os.Remove(destination)
		return false, err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		_ = os.Remove(destination)
		return false, err
	}
	return true, output.Close()
}

func cleanLabel(value string, limit int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, strings.TrimSpace(value))
	if len([]rune(value)) > limit {
		value = string([]rune(value)[:limit])
	}
	return value
}

func safeSegment(value string) string {
	var result strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			result.WriteRune(r)
		case unicode.IsSpace(r):
			result.WriteRune('-')
		}
	}
	cleaned := strings.Trim(result.String(), "-._")
	if cleaned == "" {
		return "未分類"
	}
	return cleaned
}

func mergeTags(userTags, automatic []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, group := range [][]string{userTags, automatic} {
		for _, value := range group {
			value = cleanLabel(value, 80)
			key := strings.ToLower(value)
			if value != "" && !seen[key] {
				seen[key] = true
				result = append(result, value)
			}
		}
	}
	if len(result) > 20 {
		result = result[:20]
	}
	sort.Strings(result)
	return result
}

func mediaURL(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(relative, "..") {
		return ""
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return "/media/" + strings.Join(parts, "/")
}
