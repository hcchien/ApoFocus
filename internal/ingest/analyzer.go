package ingest

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
		client:  &http.Client{Timeout: 5 * time.Minute},
	}
}

func (a *HTTPAnalyzer) Analyze(ctx context.Context, path, thumbnailPath string) (Analysis, error) {
	payload, err := json.Marshal(map[string]any{"path": path, "thumbnailPath": thumbnailPath})
	if err != nil {
		return Analysis{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/analyze", bytes.NewReader(payload))
	if err != nil {
		return Analysis{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return Analysis{}, fmt.Errorf("call embedding service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(response.Body).Decode(&body)
		return Analysis{}, fmt.Errorf("embedding service returned %s: %s", response.Status, body.Detail)
	}
	var body struct {
		Vector        []float32          `json:"vector"`
		Tags          []string           `json:"tags"`
		DominantColor string             `json:"dominantColor"`
		TimingsMS     map[string]float64 `json:"timingsMs"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		return Analysis{}, fmt.Errorf("decode embedding service response: %w", err)
	}
	if len(body.Vector) != 512 {
		return Analysis{}, fmt.Errorf("embedding service returned %d dimensions; expected 512", len(body.Vector))
	}
	return Analysis{Tags: body.Tags, Embedding: body.Vector, DominantColor: body.DominantColor, TimingsMS: body.TimingsMS}, nil
}

func (a *HTTPAnalyzer) AnalyzeBatch(ctx context.Context, inputs []AnalyzeInput) ([]BatchAnalysis, error) {
	payload, err := json.Marshal(map[string]any{"items": inputs})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/analyze-batch", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("call batch embedding service: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var body struct {
			Detail string `json:"detail"`
		}
		_ = json.NewDecoder(response.Body).Decode(&body)
		return nil, fmt.Errorf("batch embedding service returned %s: %s", response.Status, body.Detail)
	}
	var body struct {
		Items []struct {
			Path          string             `json:"path"`
			Vector        []float32          `json:"vector"`
			Tags          []string           `json:"tags"`
			DominantColor string             `json:"dominantColor"`
			TimingsMS     map[string]float64 `json:"timingsMs"`
		} `json:"items"`
	}
	if err = json.NewDecoder(response.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]BatchAnalysis, 0, len(body.Items))
	for _, item := range body.Items {
		if len(item.Vector) != 512 {
			return nil, fmt.Errorf("batch embedding returned %d dimensions", len(item.Vector))
		}
		out = append(out, BatchAnalysis{Path: item.Path, Analysis: Analysis{Embedding: item.Vector, Tags: item.Tags, DominantColor: item.DominantColor, TimingsMS: item.TimingsMS}})
	}
	return out, nil
}
