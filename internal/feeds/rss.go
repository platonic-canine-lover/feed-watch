package feeds

import (
	"encoding/xml"
	"io"
)

// rssItem is the part of an item we read. RSS 2.0 dates items with pubDate;
// RSS 1.0 (RDF), and some 2.0 feeds, use Dublin Core's dc:date instead.
type rssItem struct {
	PubDate string `xml:"pubDate"`
	DCDate  string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

var rssFeed = xmlFeed{
	name: "an RSS document",
	root: func(n xml.Name) bool { return n.Local == "rss" || n.Local == "RDF" },
	item: "item",
	itemDate: func(d *xml.Decoder, start *xml.StartElement) (string, error) {
		var it rssItem
		err := d.DecodeElement(&it, start)
		return firstNonBlank(it.PubDate, it.DCDate), err
	},
}

// ParseRSS reads an RSS 2.0 or RSS 1.0 (RDF) document and reports its items'
// dates.
//
// The channel's own lastBuildDate is ignored on purpose. Feeds rebuild on a
// timer and stamp it whether or not anything is new, so a stalled feed can
// carry today's lastBuildDate over two-month-old items.
func ParseRSS(r io.Reader) (Result, error) { return parseXMLItems(r, rssFeed) }
