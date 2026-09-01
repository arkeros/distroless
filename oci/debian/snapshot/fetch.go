package snapshot

import (
	"fmt"
	"io"
	"net/http"
	"time"
)

// FetchLatestSnapshot returns the most recent snapshot timestamp published in
// the given archive.
func FetchLatestSnapshot(archiveURL string) (string, error) {
	return fetchLatestSnapshotAt(archiveURL, time.Now().UTC())
}

// fetchLatestSnapshotAt is FetchLatestSnapshot with the clock supplied, so the
// month boundary is testable.
//
// snapshot.debian.org indexes by month, and the current month is legitimately
// empty for a while: `debian-security` publishes far less often than `debian`,
// so early on the first of a month its index either 404s or lists nothing
// while `debian` already has that day's snapshot. Falling back to the previous
// month is the difference between "no snapshot this month yet" and "no
// snapshot", which are not the same claim.
func fetchLatestSnapshotAt(archiveURL string, now time.Time) (string, error) {
	months := []time.Time{now, previousMonth(now)}

	for _, month := range months {
		timestamp, err := fetchMonth(archiveURL, month)
		if err != nil {
			return "", err
		}
		if timestamp != "" {
			return timestamp, nil
		}
	}

	return "", fmt.Errorf("no snapshot timestamps at %s for %s or %s",
		archiveURL, months[0].Format("2006-01"), months[1].Format("2006-01"))
}

// previousMonth steps back one calendar month.
//
// Deliberately not `AddDate(0, -1, 0)`: that normalises out-of-range days
// forward, so on 31 March it yields 3 March and would re-query the month it
// was trying to leave.
func previousMonth(t time.Time) time.Time {
	firstOfMonth := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	return firstOfMonth.AddDate(0, 0, -1)
}

// fetchMonth returns the latest timestamp indexed for one month, or the empty
// string when that month has none. A missing index and an index listing
// nothing mean the same thing; anything else is an error, so a broken archive
// is never mistaken for a quiet one.
func fetchMonth(archiveURL string, month time.Time) (string, error) {
	url := fmt.Sprintf("%s?year=%d&month=%d", archiveURL, month.Year(), int(month.Month()))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	matches := timestampRegex.FindAllString(string(body), -1)
	if len(matches) == 0 {
		return "", nil
	}

	// The index is ordered oldest first, so the most recent is last.
	return matches[len(matches)-1], nil
}
