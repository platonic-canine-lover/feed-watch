package feeds

import (
	"testing"
	"time"
)

func TestParseDate(t *testing.T) {
	utc := func(s string) time.Time {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			panic(err)
		}
		return t.UTC()
	}
	tests := []struct {
		name string
		in   string
		want time.Time // zero means "not a date"
	}{
		{"RFC 1123, numeric zone", "Sat, 07 Nov 2026 15:00:01 +0000", utc("2026-11-07T15:00:01Z")},
		{"GMT", "Sat, 07 Nov 2026 15:00:01 GMT", utc("2026-11-07T15:00:01Z")},
		{"one-digit day", "Sat, 7 Nov 2026 15:00:01 GMT", utc("2026-11-07T15:00:01Z")},
		{"no seconds", "Sat, 07 Nov 2026 15:00 +0000", utc("2026-11-07T15:00:00Z")},
		{"no weekday", "07 Nov 2026 15:00:01 +0000", utc("2026-11-07T15:00:01Z")},
		{"two-digit year", "Sat, 07 Nov 26 15:00:01 +0000", utc("2026-11-07T15:00:01Z")},
		{"offset converted to UTC", "Sat, 07 Nov 2026 15:00:01 -0500", utc("2026-11-07T20:00:01Z")},
		// The case time.Parse gets silently wrong: "EST" outside the US
		// parses as +0000. It must be five hours later in UTC.
		{"named US zone", "Sat, 07 Nov 2026 15:00:01 EST", utc("2026-11-07T20:00:01Z")},
		{"named zone, lower case", "Sat, 07 Nov 2026 15:00:01 pdt", utc("2026-11-07T22:00:01Z")},
		{"BST", "Sat, 07 Nov 2026 15:00:01 BST", utc("2026-11-07T14:00:01Z")},
		{"extra whitespace", "  Sat,  07 Nov 2026\n15:00:01   GMT  ", utc("2026-11-07T15:00:01Z")},
		{"ISO 8601 / dc:date", "2026-11-07T15:00:01Z", utc("2026-11-07T15:00:01Z")},
		{"ISO 8601 with offset", "2026-11-07T15:00:01+01:00", utc("2026-11-07T14:00:01Z")},
		{"ISO with fraction", "2026-09-11T21:28:33.058Z", utc("2026-09-11T21:28:33.058Z")},
		{"ISO without a zone", "2026-11-07T15:00:01", utc("2026-11-07T15:00:01Z")},
		{"date only", "2026-11-07", utc("2026-11-07T00:00:00Z")},
		{"empty", "", time.Time{}},
		{"words", "yesterday", time.Time{}},
		{"impossible day", "Sat, 32 Nov 2026 15:00:01 GMT", time.Time{}},
		{"year 0 (fuzzer find)", "0000-01-01", time.Time{}},
		{"Unix epoch placeholder", "Thu, 01 Jan 1970 00:00:00 +0000", time.Time{}},
		{"1999 is fine", "Fri, 01 Jan 1999 00:00:00 +0000", utc("1999-01-01T00:00:00Z")},
		{"unknown zone name", "Sat, 07 Nov 2026 15:00:01 XYZ", time.Time{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseDate(tt.in)
			if ok != !tt.want.IsZero() || !got.Equal(tt.want) {
				t.Fatalf("ParseDate(%q) = %v, %v; want %v", tt.in, got, ok, tt.want)
			}
			if ok && got.Location() != time.UTC {
				t.Errorf("location = %v, want UTC", got.Location())
			}
		})
	}
}
