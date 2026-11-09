# feed-watch

A small service that watches a list of RSS, Atom and JSON feeds and tells
you which ones have stopped moving.

No dependencies outside the Go standard library.

```
$ feedwatch
time=2026-11-09T13:13:28Z level=INFO msg="feedwatch listening" addr=127.0.0.1:3102

$ curl -s 127.0.0.1:3102/healthz
{"status":"ok","uptimeSeconds":42}
```

> **Status: early.** The service skeleton is built and tested (the HTTP
> server, the health endpoint, loopback-only binding, graceful shutdown),
> and so are the RSS, Atom and JSON parsers. Fetching and checking feeds on a schedule are
> being added in stages; see [Roadmap](#roadmap). This README says plainly
> what exists and what is planned.

## Why

A feed that dies loudly is easy to notice: the fetch fails, something logs
an error. The dangerous failure is the quiet one. A feed keeps answering
`200 OK` with perfectly valid XML, and the newest item in it is two months
old. Everything that consumes it carries on, because nothing is wrong
except that nothing is new.

It gets worse when feeds are combined. If one source has stalled and
another sits beside it publishing daily, a freshness check on the merged
result sees today's items and reports all is well. The stalled source can
go unnoticed for weeks.

A real example, from the day the parser was written: 9to5Mac's Elon Musk
topic feed (`https://9to5mac.com/guides/elon-musk/feed/`) answered
`200 OK` with 62 well-formed items. The newest was 159 days old.

The only reliable test is per source: **when was this feed's newest item,
and is that older than this feed normally goes quiet?** That is the whole
job of this service.

## What it will do

1. Read a sources file: each feed's name, URL, kind (RSS, Atom or a JSON
   API) and how many days of silence counts as stalled for that feed.
2. On an interval, fetch every source concurrently through a bounded worker
   pool, each fetch with its own timeout and byte cap.
3. Find the newest dated item in each response and classify the source:

   | Status | Meaning |
   |---|---|
   | `ok` | the newest item is within the feed's threshold |
   | `stale` | it answered, but its newest item is older than the threshold |
   | `failing` | HTTP error, timeout, or a body that would not parse |
   | `empty` | it parsed, but no item carried a usable date |

4. Report over HTTP, and raise an alert when a source **changes** status
   (`ok` → `stale`, `failing` → `ok`, and so on), not on every check.

## What it does not do

It does not read, store or republish feed content beyond the dates it
needs. It is a monitor, not an aggregator.

It does not accept connections from the network. The HTTP server binds to
loopback only, refuses to start on any other address, and only reads.

## Roadmap

| Stage | | |
|---|---|---|
| 1 | ✅ | Skeleton: `/healthz`, loopback-only binding, graceful shutdown |
| 2 | ✅ | Parsers for RSS (2.0, 1.0/RDF), Atom (1.0, 0.3) and JSON (JSON Feed or any named list), with table tests, fuzz tests, a benchmark and opt-in live tests |
| 3 | planned | A guarded HTTP fetcher: timeouts, byte cap, redirect rules |
| 4 | planned | Concurrent checking with a bounded worker pool; status classification |
| 5 | planned | Per-source history and the `/sources` API |
| 6 | planned | Alerts on status change, to a webhook |
| 7 | planned | A sources file, a first live run, a service definition |
| 8 | planned | `/metrics` |

## Running it

```sh
go build -o feedwatch ./cmd/feedwatch
./feedwatch
```

Stop it with Ctrl-C or `SIGTERM`: it stops accepting connections and gives
requests already in progress time to finish. A second Ctrl-C ends it
immediately.

### Configuration

| Flag | Environment | Default | Meaning |
|---|---|---|---|
| `-listen` | `FEEDWATCH_LISTEN` | `127.0.0.1:3102` | address to serve on; must be loopback |
| `-shutdown-timeout` | — | `10s` | how long in-flight requests get on shutdown |

### Exit status

| Code | Meaning |
|---|---|
| `0` | stopped cleanly after a signal |
| `1` | the service failed: the port was taken, or shutdown ran out of time |
| `2` | it was started wrongly: an unknown flag, or a non-loopback address |

## Endpoints

Available now:

- `GET /healthz` — `{"status":"ok","uptimeSeconds":N}`. The process is up
  and serving. `HEAD` is answered like `GET`; any other method gets `405`.

Planned:

- `GET /sources` — every source with its status, newest item, age and
  last error
- `GET /sources/{name}` — one source and its recent checks
- `GET /metrics` — plain-text counters

`/healthz` deliberately says nothing about the feeds. A stalled feed is a
problem with the feed, not with this service, and folding the two together
would make a quiet source look like a dead process to whatever supervises
it.

## Reading feeds

`internal/feeds` reads a feed only far enough to find its items' dates.
The RSS parser was checked against 443 real feed URLs; every failure left
was a real one (HTML error pages, rate limits, Atom feeds, a feed whose
dates are all blank).

| Format | Items | Item date |
|---|---|---|
| RSS 2.0, 1.0 (RDF) | `<item>` | `pubDate`, else `dc:date` |
| Atom 1.0, 0.3 | `<entry>` | `published`, else `updated` (0.3: `issued`, `modified`) |
| JSON Feed | `items` | `date_published`, else `date_modified` |
| Any JSON API | an array at a dot path, e.g. `data.articles` | a named field, e.g. `meta.published`; a string, or Unix seconds or milliseconds |

- **Item dates only.** RSS's `lastBuildDate` and Atom's feed-level
  `<updated>` are ignored: feeds rebuild on a timer and stamp them whether
  or not anything is new.
- **Published before updated.** The question is when something new
  appeared; an Atom entry's `updated` also moves when an old post is
  corrected.
- **Streaming XML.** Items are decoded one at a time, wherever they sit,
  so RSS 2.0 (items inside `<channel>`), RSS 1.0 and Atom share one loop.
- **Everything in UTC.** Every date comes back in UTC, and a date with no
  zone is read as UTC. The service itself runs in UTC whatever the host's
  zone, so log timestamps and "now" never shift with daylight saving.
- **Dates as feeds really write them.** One-digit days, no seconds, no
  weekday, two-digit years, ISO 8601. Named zones such as `EST` are
  converted by hand, because `time.Parse` silently reads an unfamiliar zone
  abbreviation as `+0000`. Dates before 1990 (the Unix epoch, year 0) are
  placeholders and count as undated.
- **Lenient where real feeds are untidy.** HTML entities like `&nbsp;`, a
  declared ISO-8859-1 or Windows-1252 charset, and stray control characters
  pasted in from word processors do not fail the feed.
- **An HTML page is not a feed.** A blocked request often returns an HTML
  page with `200 OK`; that is `ErrNotFeed`, never "a feed with no items".
  So is RSS handed to the Atom parser, and JSON with no array where the
  items should be.

## Design notes

**Loopback only, enforced.** `-listen :3102` would mean every interface, so
the address is checked before anything listens: `127.0.0.0/8`, `::1` and
`localhost` are accepted and everything else is refused with exit status 2.

**Every connection is bounded.** Header, read, write and idle timeouts are
set on the server, so a slow or idle client cannot hold a connection open
indefinitely.

**Shutdown is a context.** `SIGINT` and `SIGTERM` cancel the root context
(`signal.NotifyContext`); the server notices, stops accepting connections,
and lets in-flight requests finish within `-shutdown-timeout`. If they do
not, that is reported as a failure rather than hidden.

**`main` is a thin wrapper.** The work happens in `run`, which takes a
context, arguments and a log writer and returns an exit status, so the
tests can drive startup, bad usage and shutdown without starting a
process.

**Structured logs.** `log/slog`, key=value text, to stderr, timestamps in
UTC.

## Development

```sh
go vet ./...
go test -race ./...
go test -run=^$ -fuzz=FuzzParseRSS -fuzztime=60s ./internal/feeds    # also FuzzParseAtom, FuzzParseJSON
go test -run=^$ -bench=. -benchmem ./internal/feeds
FEEDWATCH_LIVE=1 go test -run Live -v ./internal/feeds   # reaches the internet
```

Inputs the fuzzer has found to fail are kept in `testdata/fuzz` and rerun
by every `go test`.

Go 1.22 or later (the HTTP routes use method patterns such as
`GET /healthz`).

```
cmd/feedwatch/     startup, flags, signal handling, graceful shutdown
internal/api/      HTTP handlers
internal/feeds/    feed parsers and date reading
```

## Licence

MIT. See [LICENSE](LICENSE).
