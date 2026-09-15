package tarballs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// httpClient bounds every upstream call. Without a timeout a stalled
// connection would hang the daily update job until the runner's own
// six-hour limit; a minute is generous for a JSON index or a checksum
// file and small next to the job's schedule.
var httpClient = &http.Client{Timeout: time.Minute}

// get fetches url. Each decorate function is applied to the request before
// it is sent — how the GitHub helper attaches its token without every
// caller having to know about one.
func get(ctx context.Context, url string, decorate ...func(*http.Request)) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	for _, d := range decorate {
		d(req)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return resp.Body, nil
}

func getJSON(ctx context.Context, url string, v any, decorate ...func(*http.Request)) error {
	body, err := get(ctx, url, decorate...)
	if err != nil {
		return err
	}
	defer body.Close()
	if err := json.NewDecoder(body).Decode(v); err != nil {
		return fmt.Errorf("decode %s: %w", url, err)
	}
	return nil
}
