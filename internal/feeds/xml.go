package feeds

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// xmlFeed describes one XML format to parseXMLItems.
type xmlFeed struct {
	name     string                                                // for errors: "an RSS document"
	root     func(xml.Name) bool                                   // is this the right root element?
	item     string                                                // local name of an item element
	itemDate func(*xml.Decoder, *xml.StartElement) (string, error) // decode one item, return its raw date
}

// parseXMLItems streams a document, decoding each item element wherever it
// sits. Streaming keeps memory flat and covers layouts where items are
// nested (RSS 2.0's <channel>) or not (RSS 1.0, Atom) without a struct for
// each.
func parseXMLItems(r io.Reader, f xmlFeed) (Result, error) {
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
				return Result{}, fmt.Errorf("%w: not %s: %v", ErrNotFeed, f.name, err)
			}
			return res, fmt.Errorf("reading %s: %w", f.name, err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if !rootSeen {
			rootSeen = true
			if !f.root(start.Name) {
				return Result{}, fmt.Errorf("%w: not %s: root element is <%s>", ErrNotFeed, f.name, start.Name.Local)
			}
			continue
		}
		if start.Name.Local != f.item {
			continue
		}
		raw, err := f.itemDate(d, &start)
		if err != nil {
			return res, fmt.Errorf("reading %s item %d: %w", f.name, res.Items+1, err)
		}
		res.add(raw)
	}
	if !rootSeen {
		return Result{}, fmt.Errorf("%w: not %s: empty body", ErrNotFeed, f.name)
	}
	return res, nil
}

// firstNonBlank returns the first argument that is not empty or spaces.
func firstNonBlank(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
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
