package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"audiax/internal/apperr"
)

// ObjectStore writes audio objects to Supabase Storage over its REST API.
//
// REST rather than an SDK: the operation needed is a single authenticated POST,
// and a dependency would not earn its place for that.
//
// The bucket must be private. It holds workplace recordings, and the uploaded
// file is raw -- the privacy notch filter lives in the AI service's inference
// path, not here, so nothing has been stripped from what is stored
// (docs/prd.md §5.4).
type ObjectStore struct {
	baseURL    string
	serviceKey string
	bucket     string
	client     *http.Client
}

func NewObjectStore(supabaseURL, serviceKey, bucket string, timeout time.Duration) *ObjectStore {
	return &ObjectStore{
		baseURL:    supabaseURL,
		serviceKey: serviceKey,
		bucket:     bucket,
		client:     &http.Client{Timeout: timeout},
	}
}

func (s *ObjectStore) Put(ctx context.Context, path string, content []byte, contentType string) error {
	url := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(content))
	if err != nil {
		return fmt.Errorf("build storage request: %w", err)
	}
	// The service_role key bypasses RLS. It never leaves the server.
	req.Header.Set("Authorization", "Bearer "+s.serviceKey)
	req.Header.Set("Content-Type", contentType)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: put object %s: %v", apperr.ErrUnavailable, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
	if resp.StatusCode >= http.StatusInternalServerError {
		return fmt.Errorf("%w: storage returned %d for %s: %s",
			apperr.ErrUnavailable, resp.StatusCode, path, string(body))
	}
	return fmt.Errorf("storage returned %d for %s: %s", resp.StatusCode, path, string(body))
}
