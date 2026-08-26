package mediaingest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type HTTPAnalyzer struct {
	baseURL string
	client  *http.Client
}

func NewHTTPAnalyzer(baseURL string) *HTTPAnalyzer {
	return &HTTPAnalyzer{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: 12 * time.Hour},
	}
}

func (a *HTTPAnalyzer) AnalyzeMedia(ctx context.Context, path, thumbnailPath, segmentDir string, autoTags bool) (Analysis, error) {
	payload, err := json.Marshal(map[string]any{
		"path": path, "thumbnailPath": thumbnailPath, "segmentDir": segmentDir, "autoTags": autoTags,
	})
	if err != nil {
		return Analysis{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/analyze-media", bytes.NewReader(payload))
	if err != nil {
		return Analysis{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return Analysis{}, fmt.Errorf("call local media analyzer: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(response.Body).Decode(&body)
		return Analysis{}, fmt.Errorf("media analyzer returned %s: %s", response.Status, body.Detail)
	}
	var result Analysis
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return Analysis{}, fmt.Errorf("decode media analyzer response: %w", err)
	}
	if result.MediaType != "video" && result.MediaType != "audio" {
		return Analysis{}, fmt.Errorf("media analyzer returned unsupported type %q", result.MediaType)
	}
	for _, segment := range result.Segments {
		if len(segment.VisualVector) != 0 && len(segment.VisualVector) != 512 {
			return Analysis{}, fmt.Errorf("visual segment %d returned %d dimensions; expected 512", segment.Index, len(segment.VisualVector))
		}
		if len(segment.AudioVector) != 0 && len(segment.AudioVector) != 512 {
			return Analysis{}, fmt.Errorf("audio segment %d returned %d dimensions; expected 512", segment.Index, len(segment.AudioVector))
		}
	}
	return result, nil
}
