package feeds

import (
	"strings"
	"time"
)

// Feeds say "RFC 822 date" and then write whatever they like: one-digit
// days, no seconds, no weekday, a named zone, ISO 8601. These are the forms
// seen in practice, most common first.
var dateLayouts = []string{
	"Mon, 2 Jan 2006 15:04:05 -0700", // RFC 1123 with a numeric zone; "2" also reads "02"
	"Mon, 2 Jan 2006 15:04 -0700",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 06 15:04:05 -0700", // two-digit year, as RFC 822 itself allowed
	time.RFC3339Nano,               // dc:date, and feeds that ignore RFC 822
	"2006-01-02T15:04:05",          // ISO without a zone: read as UTC
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// zoneOffsets turns the zone names RFC 822 allows into numeric offsets.
// time.Parse cannot do this reliably: given "EST" it only knows the offset
// if EST happens to be the machine's own zone, and otherwise returns a
// time at +0000 labelled EST — five hours wrong, with no error.
var zoneOffsets = map[string]string{
	"GMT": "+0000", "UT": "+0000", "UTC": "+0000", "Z": "+0000",
	"EST": "-0500", "EDT": "-0400",
	"CST": "-0600", "CDT": "-0500",
	"MST": "-0700", "MDT": "-0600",
	"PST": "-0800", "PDT": "-0700",
	"BST": "+0100", "CET": "+0100", "CEST": "+0200",
}

// earliestPlausible is the floor for a real item date. RSS dates from 1999;
// anything before 1990 is a placeholder (the Unix epoch, year 0) that a
// broken CMS writes when it has no date, and counting it as one would let
// a feed of placeholders look merely "old" instead of undated. Found by the
// fuzzer: "0000-01-01" parsed, then sat below time.Time's zero.
var earliestPlausible = time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)

// ParseDate reads a feed date. It returns the time in UTC and true, or the
// zero time and false when the text matches no known form or is not a
// plausible publication date.
func ParseDate(s string) (time.Time, bool) {
	s = strings.Join(strings.Fields(s), " ") // trim, and collapse runs of spaces
	if s == "" {
		return time.Time{}, false
	}
	if i := strings.LastIndexByte(s, ' '); i > 0 {
		if off, ok := zoneOffsets[strings.ToUpper(s[i+1:])]; ok {
			s = s[:i+1] + off
		}
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			if t.Before(earliestPlausible) {
				return time.Time{}, false
			}
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
