package feeds

import (
	"errors"
	"net/http"
	"os"
	"testing"
	"time"
)

// Live tests reach the real internet, so they are off by default and never
// run in CI: a site being down must not fail the build. Run them with
//
//	FEEDWATCH_LIVE=1 go test -run Live -v ./internal/feeds
func liveGet(t *testing.T, url string) *http.Response {
	t.Helper()
	if os.Getenv("FEEDWATCH_LIVE") == "" {
		t.Skip("set FEEDWATCH_LIVE=1 to run tests against real feeds")
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("User-Agent", "feedwatch-test/0 (+https://github.com/platonic-canine-lover/feed-watch)")
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("fetch %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fetch %s: HTTP %d", url, resp.StatusCode)
	}
	return resp
}

// A WordPress topic feed: 9to5Mac's Elon Musk guide. This checks the
// parsing, not the freshness, which is the classifier's job. It is also the
// project's standing example of a quiet stall: on 9 Nov 2026 it answered 200
// with 62 well-formed items, the newest from 3 Jun 2026.
func TestLiveWordPressTopicFeed(t *testing.T) {
	resp := liveGet(t, "https://9to5mac.com/guides/elon-musk/feed/")
	res, err := ParseRSS(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.Items == 0 || res.Dated != res.Items {
		t.Fatalf("items %d, dated %d: every item should carry a date", res.Items, res.Dated)
	}
	if res.Newest.Before(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)) || res.Newest.After(time.Now().Add(24*time.Hour)) {
		t.Fatalf("newest %s is not a plausible date for this feed", res.Newest)
	}
	t.Logf("%d items, newest %s (%.0f days ago)", res.Items, res.Newest.Format(time.RFC3339), time.Since(res.Newest).Hours()/24)
}

// The same guide's web page answers 200 with HTML. That must be reported as
// not a feed, never as a feed with nothing in it.
func TestLiveHTMLPageIsNotAFeed(t *testing.T) {
	resp := liveGet(t, "https://9to5mac.com/guides/elon-musk/")
	if _, err := ParseRSS(resp.Body); !errors.Is(err, ErrNotRSS) {
		t.Fatalf("error = %v, want ErrNotRSS", err)
	}
}
