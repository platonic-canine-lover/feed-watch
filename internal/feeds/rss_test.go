package feeds

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const rss2 = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel>
    <title>Example</title>
    <lastBuildDate>Mon, 09 Nov 2026 09:00:00 GMT</lastBuildDate>
    <item><title>Older</title><pubDate>Fri, 06 Nov 2026 17:10:27 GMT</pubDate></item>
    <item><title>Newest</title><pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item>
    <item><title>Middle</title><pubDate>Sat, 07 Nov 2026 00:11:27 GMT</pubDate></item>
  </channel>
</rss>`

const rss1 = `<?xml version="1.0"?>
<rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <channel rdf:about="https://example.com/"><title>Example</title></channel>
  <item rdf:about="https://example.com/a"><title>A</title><dc:date>2026-11-05T10:00:00Z</dc:date></item>
  <item rdf:about="https://example.com/b"><title>B</title><dc:date>2026-11-06T10:00:00+01:00</dc:date></item>
</rdf:RDF>`

func TestParseRSS(t *testing.T) {
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t.UTC() }
	tests := []struct {
		name    string
		body    string
		want    Result
		wantErr error // nil, ErrNotRSS, or errAny for "some error"
	}{
		{name: "RSS 2.0: newest is the latest date, not the first item", body: rss2,
			want: Result{Items: 3, Dated: 3, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "RSS 1.0 (RDF) with dc:date", body: rss1,
			want: Result{Items: 2, Dated: 2, Newest: at("2026-11-06T09:00:00Z")}},
		{name: "lastBuildDate is not an item date",
			body: `<rss><channel><lastBuildDate>Mon, 09 Nov 2026 09:00:00 GMT</lastBuildDate>
				<item><pubDate>Fri, 11 Sep 2026 00:00:00 +0000</pubDate></item></channel></rss>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-09-11T00:00:00Z")}},
		{name: "dc:date used when pubDate is missing",
			body: `<rss xmlns:dc="http://purl.org/dc/elements/1.1/"><channel><item><dc:date>2026-11-01T08:00:00Z</dc:date></item></channel></rss>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-01T08:00:00Z")}},
		{name: "undated and unreadable dates are counted but not dated",
			body: `<rss><channel><item><title>no date</title></item><item><pubDate>soon</pubDate></item>
				<item><pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>`,
			want: Result{Items: 3, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		// Every real item has a <link>; HTML calls <link> a void element, so
		// an HTML-lenient decoder once rejected 383 of 443 live feeds here.
		{name: "items with <link> elements, as every real feed has",
			body: `<rss><channel><link>https://example.com/</link><item><title>A</title><link>https://example.com/a</link>
				<pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "a stray control character in content (form feed, from a live feed)",
			body: "<rss><channel><item><description>Page\x0cbreak and a \x01</description>" +
				"<pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>",
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "empty pubDate elements (a live feed does this for all 80 items)",
			body: `<rss><channel><item><pubDate></pubDate></item><item><pubDate>  </pubDate></item></channel></rss>`,
			want: Result{Items: 2, Dated: 0}},
		{name: "a feed with no items is not an error",
			body: `<rss version="2.0"><channel><title>Quiet</title></channel></rss>`,
			want: Result{}},
		{name: "HTML entities in content do not break the feed",
			body: `<rss><channel><item><description>Price&nbsp;&pound;5 &mdash; ok</description>
				<pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "CDATA content",
			body: `<rss><channel><item><description><![CDATA[<p>Hi & bye</p>]]></description>
				<pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "legacy charset declaration",
			body: "<?xml version=\"1.0\" encoding=\"ISO-8859-1\"?><rss><channel><item><title>Caf\xe9</title>" +
				"<pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>",
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "an HTML block page with a 200 is not a feed",
			body:    `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>Checking your browser</body></html>`,
			wantErr: ErrNotRSS},
		{name: "an Atom feed is not RSS",
			body:    `<feed xmlns="http://www.w3.org/2005/Atom"><entry><updated>2026-11-07T15:00:01Z</updated></entry></feed>`,
			wantErr: ErrNotRSS},
		{name: "JSON is not RSS", body: `{"news":[]}`, wantErr: ErrNotRSS},
		{name: "empty body", body: "", wantErr: ErrNotRSS},
		{name: "unsupported charset",
			body: `<?xml version="1.0" encoding="Shift_JIS"?><rss><channel></channel></rss>`, wantErr: ErrNotRSS},
		{name: "truncated mid-item keeps the items before it",
			body: `<rss><channel><item><pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item><item><pubDate>Sun, 08`,
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}, wantErr: errAny},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseRSS(strings.NewReader(tt.body))
			switch {
			case tt.wantErr == nil && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tt.wantErr == errAny && err == nil:
				t.Fatal("expected an error")
			case tt.wantErr != nil && tt.wantErr != errAny && !errors.Is(err, tt.wantErr):
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
			if got.Items != tt.want.Items || got.Dated != tt.want.Dated || !got.Newest.Equal(tt.want.Newest) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

var errAny = errors.New("any error")

// FuzzParseRSS feeds the parser arbitrary bytes. Whatever arrives, it must
// not panic, and what it reports must be self-consistent.
func FuzzParseRSS(f *testing.F) {
	for _, seed := range []string{rss2, rss1, `<rss><channel><item><link>https://e.com/a</link><pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item></channel></rss>`, "", "<rss>", `<rss><channel><item><pubDate>x</pubDate></item>`, "<html></html>"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		res, err := ParseRSS(strings.NewReader(body))
		if errors.Is(err, ErrNotRSS) && res != (Result{}) {
			t.Fatalf("ErrNotRSS with a non-empty result %+v", res)
		}
		if res.Dated > res.Items || res.Dated < 0 {
			t.Fatalf("Dated %d, Items %d", res.Dated, res.Items)
		}
		if (res.Dated == 0) != res.Newest.IsZero() {
			t.Fatalf("Dated %d but Newest %v", res.Dated, res.Newest)
		}
	})
}

func BenchmarkParseRSS(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`<rss><channel>`)
	for i := 0; i < 50; i++ {
		sb.WriteString(`<item><title>An item</title><description>Some text &amp; more</description><pubDate>Sat, 07 Nov 2026 15:00:01 GMT</pubDate></item>`)
	}
	sb.WriteString(`</channel></rss>`)
	body := sb.String()
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := ParseRSS(strings.NewReader(body)); err != nil {
			b.Fatal(err)
		}
	}
}
