// Package feeds reads feeds just far enough to answer one question: when was
// the newest item published? It does not keep titles, links or content.
package feeds

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"
)

// Result is what a feed says about its own freshness.
type Result struct {
	Items  int       // items found
	Dated  int       // items whose date could be read
	Newest time.Time // the latest of those dates, in UTC; zero when Dated is 0
}

// ErrNotRSS means the body parsed as XML but is not an RSS document, or did
// not parse at all. A blocked fetch often returns an HTML page with a 200,
// and that has to count as a failure, not as a feed with no items.
var ErrNotRSS = errors.New("not an RSS document")

const dcNS = "http://purl.org/dc/elements/1.1/"

// rssItem is the part of an item we read. RSS 2.0 dates items with pubDate;
// RSS 1.0 (RDF), and some 2.0 feeds, use Dublin Core's dc:date instead.
type rssItem struct {
	PubDate string `xml:"pubDate"`
	DCDate  string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// ParseRSS reads an RSS 2.0 or RSS 1.0 (RDF) document and reports its items'
// dates. It streams: items are decoded one at a time, wherever they sit,
// which covers both layouts (2.0 nests items in <channel>; 1.0 puts them
// beside it) without a struct for each.
//
// The channel's own lastBuildDate is ignored on purpose. Feeds rebuild on a
// timer and stamp it whether or not anything is new, so a stalled feed can
// carry today's lastBuildDate over two-month-old items.
func ParseRSS(r io.Reader) (Result, error) {
	d := xml.NewDecoder(&controlCharFilter{src: r})
	// Real feeds are not always well-formed XML: HTML entities such as
	// &nbsp; in descriptions, an undeclared charset. Lenient decoding keeps
	// one stray &nbsp; from making a healthy feed look broken.
	//
	// Not d.AutoClose = xml.HTMLAutoClose: that list treats <link> as an
	// HTML void element, and every RSS item has <link>URL</link>. With it,
	// 383 of 443 real feeds failed on "unexpected end element </link>".
	d.Strict = false
	d.Entity = xml.HTMLEntity
	d.CharsetReader = asciiCompatible

	var res Result
	rootSeen := false
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			if !rootSeen {
				return Result{}, fmt.Errorf("%w: %v", ErrNotRSS, err)
			}
			return res, fmt.Errorf("reading RSS: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !rootSeen {
			rootSeen = true
			if name := start.Name.Local; name != "rss" && name != "RDF" {
				return Result{}, fmt.Errorf("%w: root element is <%s>", ErrNotRSS, name)
			}
			continue
		}
		if start.Name.Local != "item" {
			continue
		}
		var it rssItem
		if err := d.DecodeElement(&it, &start); err != nil {
			return res, fmt.Errorf("reading RSS item %d: %w", res.Items+1, err)
		}
		res.Items++
		raw := it.PubDate
		if strings.TrimSpace(raw) == "" {
			raw = it.DCDate
		}
		if t, ok := ParseDate(raw); ok {
			res.Dated++
			if t.After(res.Newest) {
				res.Newest = t
			}
		}
	}
	if !rootSeen {
		return Result{}, fmt.Errorf("%w: empty body", ErrNotRSS)
	}
	return res, nil
}

// asciiCompatible lets the decoder accept feeds that declare a legacy
// single-byte charset (ISO-8859-1, Windows-1252, …). The standard library
// ships no converters, and passing the bytes through is not enough: the
// decoder rejects invalid UTF-8 even inside text we ignore. So each byte is
// read as Latin-1, where byte n is code point n. For Windows-1252 that gets
// a few punctuation marks wrong (0x80–0x9F), which does not matter here:
// everything we read (element names and dates) is ASCII. Any other charset
// is refused rather than guessed at.
func asciiCompatible(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "iso-8859-1", "iso-8859-15", "latin1", "windows-1252", "cp1252", "us-ascii", "ascii":
		return &latin1Reader{src: input}, nil
	}
	return nil, fmt.Errorf("unsupported charset %q", charset)
}

// latin1Reader converts Latin-1 bytes to UTF-8 as they are read.
type latin1Reader struct {
	src     io.Reader
	pending []byte // UTF-8 output not yet handed to the caller
	buf     [512]byte
}

func (l *latin1Reader) Read(p []byte) (int, error) {
	for len(l.pending) == 0 {
		n, err := l.src.Read(l.buf[:])
		for _, b := range l.buf[:n] {
			l.pending = utf8.AppendRune(l.pending, rune(b))
		}
		if n == 0 && err != nil {
			return 0, err
		}
	}
	n := copy(p, l.pending)
	l.pending = l.pending[n:]
	return n, nil
}

// controlCharFilter replaces the control characters XML forbids (all below
// 0x20 except tab, newline and carriage return) with spaces. Go's decoder
// rejects them even when lenient, and they turn up in real feeds pasted from
// word processors: one form feed (U+000C) in one description stopped a live
// feed at its ninth item. Byte-level replacement is safe for UTF-8, where
// bytes below 0x80 never occur inside a multi-byte character.
type controlCharFilter struct{ src io.Reader }

func (f *controlCharFilter) Read(p []byte) (int, error) {
	n, err := f.src.Read(p)
	for i, b := range p[:n] {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			p[i] = ' '
		}
	}
	return n, err
}
