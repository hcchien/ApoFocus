package ingest

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
	"unicode"

	"github.com/hcchien/apofocus/internal/fileidentity"
)

const maxPhotoBytes int64 = 2 << 30

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
	if len(importRoots) == 0 {
		return nil, errors.New("at least one APOFOCUS_IMPORT_ROOTS entry is required")
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
	if analyzer == nil {
		return nil, errors.New("analyzer is required")
	}
	return &Manager{libraryRoot: root, importRoots: cleanImports, analyzer: analyzer, repository: repository}, nil
}

func (m *Manager) Inspect(ctx context.Context, request ImportRequest) (Inspection, error) {
	inspection, err := m.inspectMetadata(request)
	if err != nil {
		return Inspection{}, err
	}
	if request.AutoTags {
		analysis, err := m.analyzer.Analyze(ctx, inspection.SourcePath, "")
		if err != nil {
			return Inspection{}, err
		}
		inspection.SuggestedTags = mergeTags(request.Tags, analysis.Tags)
		inspection.DominantColor = analysis.DominantColor
		inspection.AnalysisTimings = analysis.TimingsMS
	} else {
		inspection.SuggestedTags = mergeTags(request.Tags, nil)
	}
	return inspection, nil
}

// ValidateBatchRoot resolves a requested folder and ensures it is a directory
// inside one of the configured import roots.
func (m *Manager) ValidateBatchRoot(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", errors.New("source root is required")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve source root: %w", err)
	}
	allowed := false
	for _, root := range m.importRoots {
		if pathWithin(root, resolved) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", errors.New("source root is outside APOFOCUS_IMPORT_ROOTS")
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !stat.IsDir() {
		return "", errors.New("source root must be a directory")
	}
	return resolved, nil
}

func (m *Manager) Import(ctx context.Context, request ImportRequest) (ImportResult, error) {
	if m.repository == nil {
		return ImportResult{}, errors.New("repository is required for import")
	}
	inspection, err := m.inspectMetadata(request)
	if err != nil {
		return ImportResult{}, err
	}
	if existing, found, err := m.repository.FindByHash(ctx, inspection.ContentSHA256); err != nil {
		return ImportResult{}, err
	} else if found {
		return ImportResult{PhotoID: existing.ID, Path: existing.Path, ThumbnailPath: existing.ThumbnailPath, Tags: existing.Tags, AlreadyExists: true}, nil
	}

	originalPath, thumbnailPath := m.destinationPaths(inspection)
	thumbExisted := fileExists(thumbnailPath)
	analysis, err := m.analyzer.Analyze(ctx, inspection.SourcePath, thumbnailPath)
	if err != nil {
		return ImportResult{}, err
	}
	inspection.embedding = analysis.Embedding
	inspection.DominantColor = analysis.DominantColor
	inspection.AnalysisTimings = analysis.TimingsMS
	if request.AutoTags {
		inspection.SuggestedTags = mergeTags(request.Tags, analysis.Tags)
	} else {
		inspection.SuggestedTags = mergeTags(request.Tags, nil)
	}

	createdOriginal, err := copyPhoto(inspection.SourcePath, originalPath)
	if err != nil {
		if !thumbExisted {
			_ = os.Remove(thumbnailPath)
		}
		return ImportResult{}, err
	}
	cleanup := func() {
		if createdOriginal {
			_ = os.Remove(originalPath)
		}
		if !thumbExisted {
			_ = os.Remove(thumbnailPath)
		}
	}
	originalIdentity, err := fileidentity.FromPath(originalPath)
	if err != nil {
		cleanup()
		return ImportResult{}, fmt.Errorf("identify managed photo: %w", err)
	}
	thumbnailIdentity, err := fileidentity.FromPath(thumbnailPath)
	if err != nil {
		cleanup()
		return ImportResult{}, fmt.Errorf("identify managed thumbnail: %w", err)
	}

	record := PhotoRecord{
		Inspection:            inspection,
		Path:                  originalPath,
		ThumbnailPath:         thumbnailPath,
		RelativePath:          managedRelativePath(m.libraryRoot, originalPath),
		ThumbnailRelativePath: managedRelativePath(m.libraryRoot, thumbnailPath),
		FileID:                originalIdentity.FileID,
		ThumbnailFileID:       thumbnailIdentity.FileID,
		ImageURL:              mediaURL(m.libraryRoot, originalPath),
		ThumbnailURL:          mediaURL(m.libraryRoot, thumbnailPath),
		Tags:                  inspection.SuggestedTags,
	}
	photoID, err := m.repository.Insert(ctx, record)
	if err != nil {
		cleanup()
		return ImportResult{}, err
	}
	return ImportResult{
		PhotoID: photoID, Path: originalPath, ThumbnailPath: thumbnailPath,
		Tags: record.Tags, VectorDimensions: len(inspection.embedding), AnalysisTimings: inspection.AnalysisTimings,
	}, nil
}

func managedRelativePath(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return ""
	}
	return filepath.ToSlash(relative)
}

func (m *Manager) inspectMetadata(request ImportRequest) (Inspection, error) {
	source, stat, err := m.resolveSource(request.SourcePath)
	if err != nil {
		return Inspection{}, err
	}
	if stat.Size() > maxPhotoBytes {
		return Inspection{}, fmt.Errorf("photo exceeds the %d byte limit", maxPhotoBytes)
	}
	hash, err := hashFile(source)
	if err != nil {
		return Inspection{}, err
	}
	inspection, err := readMetadata(source, stat.ModTime())
	if err != nil {
		return Inspection{}, err
	}
	inspection.SourcePath = source
	inspection.ContentSHA256 = hash
	if title := cleanLabel(request.Title, 240); title != "" {
		inspection.Title = title
	}
	inspection.Project = cleanLabel(request.Project, 120)
	if inspection.Project == "" {
		inspection.Project = "未分類"
	}
	if inspection.Location != nil {
		inspection.Location.Name = cleanLabel(request.LocationName, 160)
	}
	original, _ := m.destinationPaths(inspection)
	inspection.SuggestedFolder = filepath.Dir(original)
	return inspection, nil
}

func (m *Manager) resolveSource(value string) (string, os.FileInfo, error) {
	if strings.TrimSpace(value) == "" {
		return "", nil, errors.New("source_path is required")
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
		if pathWithin(root, resolved) {
			allowed = true
			break
		}
	}
	if !allowed {
		return "", nil, errors.New("source_path is outside APOFOCUS_IMPORT_ROOTS")
	}
	stat, err := os.Stat(resolved)
	if err != nil {
		return "", nil, err
	}
	if !stat.Mode().IsRegular() {
		return "", nil, errors.New("source_path must be a regular file")
	}
	return resolved, stat, nil
}

func (m *Manager) destinationPaths(inspection Inspection) (string, string) {
	project := safeSegment(inspection.Project)
	datePrefix := inspection.TakenAt.Format("2006-01-02")
	base := safeSegment(strings.TrimSuffix(inspection.Filename, filepath.Ext(inspection.Filename)))
	if base == "" {
		base = "photo"
	}
	extension := strings.ToLower(filepath.Ext(inspection.Filename))
	filename := fmt.Sprintf("%s_%s_%s%s", datePrefix, base, inspection.ContentSHA256[:10], extension)
	thumbnail := strings.TrimSuffix(filename, extension) + ".avif"
	year := fmt.Sprintf("%04d", inspection.Year)
	return filepath.Join(m.libraryRoot, "originals", year, project, filename),
		filepath.Join(m.libraryRoot, "thumbnails", year, project, thumbnail)
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

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
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

func copyPhoto(source, destination string) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o750); err != nil {
		return false, err
	}
	input, err := os.Open(source)
	if err != nil {
		return false, err
	}
	defer input.Close()
	output, err := os.CreateTemp(filepath.Dir(destination), ".apofocus-copy-*")
	if err != nil {
		return false, err
	}
	temporary := output.Name()
	defer os.Remove(temporary)
	if err := output.Chmod(0o640); err != nil {
		_ = output.Close()
		return false, err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return false, err
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return false, err
	}
	if err := output.Close(); err != nil {
		return false, err
	}
	if _, err := os.Stat(destination); err == nil {
		existingHash, hashErr := hashFile(destination)
		sourceHash, sourceErr := hashFile(source)
		if hashErr == nil && sourceErr == nil && existingHash == sourceHash {
			return false, nil
		}
		return false, errors.New("destination exists with different content")
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.Rename(temporary, destination); err != nil {
		return false, err
	}
	return true, nil
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

func mergeTags(userTags, automatic []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(userTags)+len(automatic))
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
