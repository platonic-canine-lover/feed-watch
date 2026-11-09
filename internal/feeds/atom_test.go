package feeds

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const atom10 = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example</title>
  <updated>2026-11-09T09:00:00Z</updated>
  <link href="https://example.com/"/>
  <entry><title>A</title><link rel="alternate" href="https://example.com/a"/>
    <published>2026-11-05T10:00:00Z</published><updated>2026-11-08T10:00:00Z</updated></entry>
  <entry><title>B</title><published>2026-11-06T10:00:00-05:00</published><updated>2026-11-06T15:00:00Z</updated></entry>
</feed>`

func TestParseAtom(t *testing.T) {
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t.UTC() }
	tests := []struct {
		name    string
		body    string
		want    Result
		wantErr bool
	}{
		// A was corrected on the 8th, but B is the newest PUBLISHED entry.
		{name: "published wins over updated", body: atom10,
			want: Result{Items: 2, Dated: 2, Newest: at("2026-11-06T15:00:00Z")}},
		{name: "updated when published is absent",
			body: `<feed xmlns="http://www.w3.org/2005/Atom"><entry><updated>2026-11-07T10:00:00Z</updated></entry></feed>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T10:00:00Z")}},
		{name: "feed-level updated is not an entry date",
			body: `<feed xmlns="http://www.w3.org/2005/Atom"><updated>2026-11-09T09:00:00Z</updated>
				<entry><published>2026-09-11T00:00:00Z</published></entry></feed>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-09-11T00:00:00Z")}},
		{name: "Atom 0.3: issued and modified",
			body: `<feed version="0.3" xmlns="http://purl.org/atom/ns#"><entry><issued>2026-11-01T08:00:00Z</issued>
				<modified>2026-11-03T08:00:00Z</modified></entry></feed>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-01T08:00:00Z")}},
		{name: "a link element inside entries does not break parsing",
			body: `<feed xmlns="http://www.w3.org/2005/Atom"><entry><link href="https://e.com/a"/><link rel="enclosure" href="x"></link>
				<published>2026-11-07T10:00:00Z</published></entry></feed>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T10:00:00Z")}},
		{name: "no entries", body: `<feed xmlns="http://www.w3.org/2005/Atom"><title>Quiet</title></feed>`, want: Result{}},
		{name: "RSS is not Atom", body: rss2, wantErr: true},
		{name: "<feed> without the Atom namespace is not Atom",
			body: `<feed><entry><published>2026-11-07T10:00:00Z</published></entry></feed>`, wantErr: true},
		{name: "HTML is not Atom", body: `<!DOCTYPE html><html><body>blocked</body></html>`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseAtom(strings.NewReader(tt.body))
			if tt.wantErr {
				if !errors.Is(err, ErrNotFeed) {
					t.Fatalf("error = %v, want ErrNotFeed", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Items != tt.want.Items || got.Dated != tt.want.Dated || !got.Newest.Equal(tt.want.Newest) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func FuzzParseAtom(f *testing.F) {
	for _, seed := range []string{atom10, "", "<feed>", `<feed xmlns="http://www.w3.org/2005/Atom"><entry>`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		checkInvariants(t, func(r *strings.Reader) (Result, error) { return ParseAtom(r) }, body)
	})
}
