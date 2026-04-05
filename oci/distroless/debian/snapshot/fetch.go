package snapshot

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// FetchLatestSnapshot fetches the latest snapshot timestamp from the given archive URL.
func FetchLatestSnapshot(archiveURL string) (string, error) {
	now := time.Now().UTC()
	url := fmt.Sprintf("%s?year=%d&month=%d", archiveURL, now.Year(), int(now.Month()))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	matches := timestampRegex.FindAllString(string(body), -1)
	if len(matches) == 0 {
		return "", fmt.Errorf("no snapshot timestamps found at %s", url)
	}

	// Return the last (most recent) timestamp
	return matches[len(matches)-1], nil
}
