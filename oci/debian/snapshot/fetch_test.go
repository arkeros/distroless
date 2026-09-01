package snapshot

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// archive serves a snapshot index keyed by "year-month", 404ing for any month
// it was not given — which is what snapshot.debian.org does for a month with
// no snapshots yet.
func archive(t *testing.T, byMonth map[string]string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("year") + "-" + r.URL.Query().Get("month")
		body, ok := byMonth[key]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, body)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/"
}

func TestFetchLatestSnapshotUsesTheCurrentMonth(t *testing.T) {
	url := archive(t, map[string]string{
		"2026-9": `<a href="20260901T022952Z/">20260901T022952Z</a>`,
	})

	got, err := fetchLatestSnapshotAt(url, time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchLatestSnapshotAt: %v", err)
	}
	if got != "20260901T022952Z" {
		t.Errorf("got %q, want 20260901T022952Z", got)
	}
}

// What broke on 2026-09-01: `debian-security` publishes far less often than
// `debian`, so early in a month its index for that month does not exist yet.
func TestFetchLatestSnapshotFallsBackWhenTheMonthIsMissing(t *testing.T) {
	url := archive(t, map[string]string{
		"2026-8": `<a href="20260830T090000Z/">20260830T090000Z</a>`,
	})

	got, err := fetchLatestSnapshotAt(url, time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchLatestSnapshotAt: %v", err)
	}
	if got != "20260830T090000Z" {
		t.Errorf("got %q, want the previous month's 20260830T090000Z", got)
	}
}

// An index that exists but lists nothing means the same thing as a missing
// one, and has to fall back too.
func TestFetchLatestSnapshotFallsBackWhenTheMonthIsEmpty(t *testing.T) {
	url := archive(t, map[string]string{
		"2026-9": `<html><body>no snapshots</body></html>`,
		"2026-8": `<a href="20260830T090000Z/">20260830T090000Z</a>`,
	})

	got, err := fetchLatestSnapshotAt(url, time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchLatestSnapshotAt: %v", err)
	}
	if got != "20260830T090000Z" {
		t.Errorf("got %q, want 20260830T090000Z", got)
	}
}

// Stepping back a month must not be done by subtracting 30 days or by
// `AddDate(0, -1, 0)`, which on the 31st normalises forward into the same
// month again and would re-query March instead of February.
func TestFetchLatestSnapshotStepsBackFromAShortMonth(t *testing.T) {
	url := archive(t, map[string]string{
		"2026-2": `<a href="20260228T120000Z/">20260228T120000Z</a>`,
	})

	got, err := fetchLatestSnapshotAt(url, time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("fetchLatestSnapshotAt: %v", err)
	}
	if got != "20260228T120000Z" {
		t.Errorf("got %q, want February's 20260228T120000Z", got)
	}
}

func TestFetchLatestSnapshotReportsTwoEmptyMonths(t *testing.T) {
	url := archive(t, nil)

	if _, err := fetchLatestSnapshotAt(url, time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)); err == nil {
		t.Error("no error when neither month has snapshots")
	}
}

// A server error is not "no snapshots this month" and must not be swallowed by
// the fallback.
func TestFetchLatestSnapshotDoesNotFallBackOnServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	if _, err := fetchLatestSnapshotAt(server.URL+"/", time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)); err == nil {
		t.Error("no error on HTTP 500")
	}
}
