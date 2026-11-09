package feeds

import (
	"encoding/xml"
	"io"
)

const (
	atom10NS = "http://www.w3.org/2005/Atom"
	atom03NS = "http://purl.org/atom/ns#"
)

// atomEntry is the part of an entry we read. Atom 1.0 has published
// (optional) and updated (required); the older 0.3 called them issued and
// modified. Unqualified tags match whichever namespace the feed uses.
type atomEntry struct {
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
	Issued    string `xml:"issued"`
	Modified  string `xml:"modified"`
}

var atomFeed = xmlFeed{
	name: "an Atom feed",
	root: func(n xml.Name) bool { return n.Local == "feed" && (n.Space == atom10NS || n.Space == atom03NS) },
	item: "entry",
	itemDate: func(d *xml.Decoder, start *xml.StartElement) (string, error) {
		var e atomEntry
		err := d.DecodeElement(&e, start)
		// Published first: the question is when something NEW appeared, and
		// updated also moves when an old entry is corrected. Updated only
		// when there is no published date, which Atom allows.
		return firstNonBlank(e.Published, e.Issued, e.Updated, e.Modified), err
	},
}

// ParseAtom reads an Atom 1.0 or 0.3 feed and reports its entries' dates.
// Like lastBuildDate in RSS, the feed-level <updated> is ignored.
func ParseAtom(r io.Reader) (Result, error) { return parseXMLItems(r, atomFeed) }
