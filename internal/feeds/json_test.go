package feeds

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const jsonFeed = `{
  "version": "https://jsonfeed.org/version/1.1",
  "title": "Example",
  "items": [
    {"id": "1", "date_published": "2026-11-05T10:00:00Z"},
    {"id": "2", "date_published": "2026-11-06T10:00:00+01:00"},
    {"id": "3", "date_modified": "2026-11-07T10:00:00Z"},
    {"id": "4"}
  ]
}`

// Shaped like a real news API: items under "news", dated by a plain day.
const newsAPI = `{"news":[
  {"title":"Big news","date_universal":"2026-11-05","uri":"/a"},
  {"title":"Older","date_universal":"2026-10-29","uri":"/b"}
],"paginate":{"total_items":2}}`

func TestParseJSON(t *testing.T) {
	at := func(s string) time.Time { t, _ := time.Parse(time.RFC3339, s); return t.UTC() }
	tests := []struct {
		name            string
		body, path, key string
		want            Result
		wantNotFeed     bool
		wantErr         bool
	}{
		{name: "JSON Feed by default: date_published, then date_modified", body: jsonFeed,
			want: Result{Items: 4, Dated: 3, Newest: at("2026-11-07T10:00:00Z")}},
		{name: "a news API with a named list and a date-only field", body: newsAPI, path: "news", key: "date_universal",
			want: Result{Items: 2, Dated: 2, Newest: at("2026-11-05T00:00:00Z")}},
		{name: "nested paths", body: `{"data":{"articles":[{"meta":{"published":"Sat, 07 Nov 2026 15:00:01 GMT"}}]}}`,
			path: "data.articles", key: "meta.published",
			want: Result{Items: 1, Dated: 1, Newest: at("2026-11-07T15:00:01Z")}},
		{name: "Unix seconds", body: `{"items":[{"t":1762527601}]}`, path: "items", key: "t",
			want: Result{Items: 1, Dated: 1, Newest: time.Unix(1762527601, 0).UTC()}},
		{name: "Unix milliseconds", body: `{"items":[{"t":1762527601500}]}`, path: "items", key: "t",
			want: Result{Items: 1, Dated: 1, Newest: time.UnixMilli(1762527601500).UTC()}},
		{name: "zero, negative and epoch timestamps are not dates", body: `{"items":[{"t":0},{"t":-5},{"t":1},{"t":"1970-01-01"}]}`,
			path: "items", key: "t", want: Result{Items: 4}},
		{name: "a date of the wrong type is ignored", body: `{"items":[{"t":true},{"t":{"x":1}},{"t":null}]}`,
			path: "items", key: "t", want: Result{Items: 3}},
		{name: "an empty list is not an error", body: `{"items":[]}`, want: Result{}},
		{name: "no array at the path", body: `{"news":{"a":1}}`, path: "news", key: "d", wantNotFeed: true},
		{name: "missing path", body: newsAPI, path: "articles", key: "d", wantNotFeed: true},
		{name: "HTML is not JSON", body: `<!DOCTYPE html><html></html>`, wantNotFeed: true},
		{name: "a path without a date field is a configuration error", body: newsAPI, path: "news", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseJSON(strings.NewReader(tt.body), tt.path, tt.key)
			switch {
			case tt.wantNotFeed:
				if !errors.Is(err, ErrNotFeed) {
					t.Fatalf("error = %v, want ErrNotFeed", err)
				}
				return
			case tt.wantErr:
				if err == nil || errors.Is(err, ErrNotFeed) {
					t.Fatalf("error = %v, want a configuration error", err)
				}
				return
			case err != nil:
				t.Fatal(err)
			}
			if got.Items != tt.want.Items || got.Dated != tt.want.Dated || !got.Newest.Equal(tt.want.Newest) {
				t.Fatalf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func FuzzParseJSON(f *testing.F) {
	for _, seed := range []string{jsonFeed, newsAPI, "", "{", `{"items":[{"date_published":1e400}]}`} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, body string) {
		checkInvariants(t, func(r *strings.Reader) (Result, error) { return ParseJSON(r, "", "") }, body)
	})
}

func TestParseDispatchesByKind(t *testing.T) {
	for _, tc := range []struct {
		f    Format
		body string
	}{
		{Format{Kind: RSS}, rss2},
		{Format{Kind: Atom}, atom10},
		{Format{Kind: JSON}, jsonFeed},
		{Format{Kind: JSON, ItemsPath: "news", DateField: "date_universal"}, newsAPI},
	} {
		if res, err := Parse(strings.NewReader(tc.body), tc.f); err != nil || res.Dated == 0 {
			t.Errorf("Parse(%+v) = %+v, %v", tc.f, res, err)
		}
	}
	if _, err := Parse(strings.NewReader(rss2), Format{Kind: "csv"}); err == nil {
		t.Error("an unknown kind should be an error")
	}
}
