package feeds

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"
)

// ParseJSON reads a JSON document whose items sit in an array at itemsPath,
// each dated by the field at dateField. Both are dot-separated paths
// ("data.articles", "meta.published"). Empty means JSON Feed: "items",
// dated by "date_published", falling back to "date_modified".
//
// A date may be a string in any form ParseDate reads, or a number of Unix
// seconds (or milliseconds, for values too large to be seconds).
func ParseJSON(r io.Reader, itemsPath, dateField string) (Result, error) {
	dateFields := []string{dateField}
	if itemsPath == "" && dateField == "" {
		itemsPath, dateFields = "items", []string{"date_published", "date_modified"}
	}
	if itemsPath == "" || dateFields[0] == "" {
		return Result{}, fmt.Errorf("JSON format needs both an items path and a date field")
	}

	d := json.NewDecoder(r)
	d.UseNumber() // keep timestamps exact; float64 loses millisecond precision
	var doc any
	if err := d.Decode(&doc); err != nil {
		return Result{}, fmt.Errorf("%w: not JSON: %v", ErrNotFeed, err)
	}
	list, ok := lookup(doc, itemsPath).([]any)
	if !ok {
		return Result{}, fmt.Errorf("%w: no array at %q", ErrNotFeed, itemsPath)
	}

	var res Result
	for _, item := range list {
		res.Items++
		for _, f := range dateFields {
			if t, ok := jsonDate(lookup(item, f)); ok {
				res.addTime(t)
				break
			}
		}
	}
	return res, nil
}

// lookup walks a dot-separated path through nested objects. Nil when any
// step is missing or not an object.
func lookup(v any, path string) any {
	for _, key := range strings.Split(path, ".") {
		obj, ok := v.(map[string]any)
		if !ok {
			return nil
		}
		v = obj[key]
	}
	return v
}

// jsonDate reads a date that is a string or a Unix timestamp.
func jsonDate(v any) (time.Time, bool) {
	switch x := v.(type) {
	case string:
		return ParseDate(x)
	case json.Number:
		f, err := x.Float64()
		if err != nil || math.IsInf(f, 0) || f <= 0 {
			return time.Time{}, false
		}
		// Seconds reach 1e12 only in the year 33658; anything that large is
		// milliseconds.
		var t time.Time
		if f >= 1e12 {
			t = time.UnixMilli(int64(f)).UTC()
		} else {
			t = time.Unix(int64(f), 0).UTC()
		}
		if t.Before(earliestPlausible) || t.Year() > 9999 {
			return time.Time{}, false
		}
		return t, true
	}
	return time.Time{}, false
}
