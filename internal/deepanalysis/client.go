package deepanalysis

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
		client:  &http.Client{Timeout: 15 * time.Minute},
	}
}

func (a *HTTPAnalyzer) Analyze(ctx context.Context, path string) (json.RawMessage, error) {
	payload, err := json.Marshal(map[string]string{"path": path})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+"/v1/analyze-photo", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("deep analysis service unavailable: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		var detail struct {
			Detail string `json:"detail"`
		}
		_ = json.Unmarshal(body, &detail)
		if detail.Detail == "" {
			detail.Detail = strings.TrimSpace(string(body))
		}
		return nil, fmt.Errorf("deep analysis failed (%s): %s", response.Status, detail.Detail)
	}
	if !json.Valid(body) {
		return nil, errors.New("deep analysis service returned invalid JSON")
	}
	return json.RawMessage(body), nil
}
