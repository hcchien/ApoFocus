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

type ServiceStatus struct {
	Available   bool   `json:"available"`
	Active      bool   `json:"active"`
	Model       string `json:"model"`
	ModelLoaded bool   `json:"modelLoaded"`
}

func (a *HTTPAnalyzer) Status(ctx context.Context) (ServiceStatus, error) {
	if a == nil || a.baseURL == "" {
		return ServiceStatus{Available: false, Active: false}, nil
	}
	reqCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, http.MethodGet, a.baseURL+"/healthz", nil)
	if err != nil {
		return ServiceStatus{Available: false, Active: false}, err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return ServiceStatus{Available: false, Active: false}, nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ServiceStatus{Available: false, Active: false}, nil
	}
	var data struct {
		Active      bool   `json:"active"`
		Model       string `json:"model"`
		ModelLoaded bool   `json:"modelLoaded"`
	}
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return ServiceStatus{Available: true, Active: true}, nil
	}
	return ServiceStatus{
		Available:   true,
		Active:      data.Active,
		Model:       data.Model,
		ModelLoaded: data.ModelLoaded,
	}, nil
}

func (a *HTTPAnalyzer) Deactivate(ctx context.Context) (ServiceStatus, error) {
	if a == nil || a.baseURL == "" {
		return ServiceStatus{Available: false, Active: false}, errors.New("deep analysis service is not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, http.MethodPost, a.baseURL+"/v1/deactivate", nil)
	if err != nil {
		return ServiceStatus{}, err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return ServiceStatus{}, fmt.Errorf("deactivate failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ServiceStatus{}, fmt.Errorf("deactivate failed with status: %s", response.Status)
	}
	return a.Status(ctx)
}

func (a *HTTPAnalyzer) Activate(ctx context.Context) (ServiceStatus, error) {
	if a == nil || a.baseURL == "" {
		return ServiceStatus{Available: false, Active: false}, errors.New("deep analysis service is not configured")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(reqCtx, http.MethodPost, a.baseURL+"/v1/activate", nil)
	if err != nil {
		return ServiceStatus{}, err
	}
	response, err := a.client.Do(request)
	if err != nil {
		return ServiceStatus{}, fmt.Errorf("activate failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return ServiceStatus{}, fmt.Errorf("activate failed with status: %s", response.Status)
	}
	return a.Status(ctx)
}
