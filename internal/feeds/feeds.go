// Package feeds reads feeds just far enough to answer one question: when was
// the newest item published? It does not keep titles, links or content.
//
// Three formats: RSS (2.0 and 1.0/RDF), Atom (1.0 and 0.3), and JSON (JSON
// Feed, or any JSON API whose item list and date field are named).
package feeds

import (
	"errors"
	"fmt"
	"io"
	"time"
)

// Result is what a feed says about its own freshness.
type Result struct {
	Items  int       // items found
	Dated  int       // items whose date could be read
	Newest time.Time // the latest of those dates, in UTC; zero when Dated is 0
}

// add counts one item and its date, if it had a readable one.
func (r *Result) add(raw string) {
	r.Items++
	if t, ok := ParseDate(raw); ok {
		r.addTime(t)
	}
}

func (r *Result) addTime(t time.Time) {
	r.Dated++
	if t.After(r.Newest) {
		r.Newest = t
	}
}

// ErrNotFeed means the body is not a feed of the expected kind: it did not
// parse, or parsed as something else. A blocked fetch often returns an HTML
// page with a 200, and that has to count as a failure, never as a feed with
// no items. The wrapped message says which kind was expected.
var ErrNotFeed = errors.New("not a feed of the expected kind")

// Kind names a feed format.
type Kind string

const (
	RSS  Kind = "rss"
	Atom Kind = "atom"
	JSON Kind = "json"
)

// Format says how to read one source. For JSON, ItemsPath and DateField
// name where the items are and which field dates them; empty means JSON
// Feed (https://jsonfeed.org): "items" and "date_published".
type Format struct {
	Kind      Kind   `json:"kind"`
	ItemsPath string `json:"itemsPath,omitempty"` // dot-separated, e.g. "data.articles"
	DateField string `json:"dateField,omitempty"` // dot-separated within an item
}

// Parse reads r in the given format.
func Parse(r io.Reader, f Format) (Result, error) {
	switch f.Kind {
	case RSS:
		return ParseRSS(r)
	case Atom:
		return ParseAtom(r)
	case JSON:
		return ParseJSON(r, f.ItemsPath, f.DateField)
	}
	return Result{}, fmt.Errorf("unknown feed kind %q", f.Kind)
}
