package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"

	"audiax/internal/apperr"
	"audiax/internal/constants"
	"audiax/internal/model"
)

// AIService is the HTTP client for the stateless inference service in
// ../audiax_model. That service holds no state of its own: it returns a baseline
// to its caller and expects the caller to send it back on every inspection.
// This backend is that caller.
type AIService struct {
	baseURL string
	client  *http.Client
}

func NewAIService(baseURL string, timeout time.Duration) *AIService {
	return &AIService{baseURL: baseURL, client: &http.Client{Timeout: timeout}}
}

// Calibrate builds a baseline from a healthy-condition recording.
//
// audio is a []byte rather than an io.Reader because the same bytes are also
// uploaded to object storage, and a reader can only be consumed once.
// constants.MaxAudioUploadBytes bounds what this can cost.
func (s *AIService) Calibrate(ctx context.Context, audio []byte, filename, machineLabel string) (*model.AIBaseline, error) {
	body, contentType, err := multipartBody(audio, filename, map[string]string{
		constants.AIMachineLabelFormField: machineLabel,
	})
	if err != nil {
		return nil, err
	}

	baseline := new(model.AIBaseline)
	if err := s.post(ctx, constants.AICalibratePath, body, contentType, baseline); err != nil {
		return nil, err
	}
	return baseline, nil
}

func (s *AIService) Inspect(ctx context.Context, audio []byte, filename string, baselineJSON []byte) (*model.AIHealthCard, error) {
	body, contentType, err := multipartBody(audio, filename, map[string]string{
		constants.AIBaselineFormField: string(baselineJSON),
	})
	if err != nil {
		return nil, err
	}

	card := new(model.AIHealthCard)
	if err := s.post(ctx, constants.AIInspectPath, body, contentType, card); err != nil {
		return nil, err
	}
	return card, nil
}

func multipartBody(audio []byte, filename string, fields map[string]string) (*bytes.Buffer, string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	for name, value := range fields {
		if err := writer.WriteField(name, value); err != nil {
			return nil, "", fmt.Errorf("write form field %s: %w", name, err)
		}
	}

	part, err := writer.CreateFormFile(constants.AudioFormField, filename)
	if err != nil {
		return nil, "", fmt.Errorf("create audio part: %w", err)
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", fmt.Errorf("write audio part: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("close multipart writer: %w", err)
	}
	return &buf, writer.FormDataContentType(), nil
}

func (s *AIService) post(ctx context.Context, path string, body *bytes.Buffer, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+path, body)
	if err != nil {
		return fmt.Errorf("build ai request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		// A transport failure is the dependency's problem, not the caller's.
		return fmt.Errorf("%w: call %s: %v", apperr.ErrUnavailable, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errorFor(path, resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode ai response from %s: %w", path, err)
	}
	return nil
}

// errorFor turns an AI service failure into the right domain error. FastAPI puts
// its message in {"detail": "..."}; for a 400 that text is the quality gate
// telling the operator what to do differently, so it must survive to the client.
func errorFor(path string, resp *http.Response) error {
	var payload struct {
		Detail string `json:"detail"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024))
	_ = json.Unmarshal(raw, &payload)

	if resp.StatusCode == http.StatusBadRequest && payload.Detail != "" {
		return &apperr.RejectedError{Reason: payload.Detail}
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: %s returned %d: %s",
			apperr.ErrUnavailable, path, resp.StatusCode, payload.Detail)
	}
	return fmt.Errorf("ai service %s returned %d: %s", path, resp.StatusCode, string(raw))
}
