// feedwatch checks a list of RSS, Atom and JSON feeds and says which ones
// have stopped moving.
//
// Why it exists: a feed that dies loudly is easy. The hard one returns a
// healthy 200 full of perfectly valid, two-month-old items. Anything that
// consumes it carries on, and if a second, fresher feed sits beside it,
// even a freshness check on the combined result says all is well. The only
// reliable test is per source: when was this feed's newest item?
//
// The HTTP server binds to loopback only and only reads. It exists so the
// service can be asked how the feeds are getting on; it is not a way in.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/platonic-canine-lover/feed-watch/internal/api"
)

func main() {
	// Everything runs in UTC, whatever the host's zone. Feed dates are
	// compared with "now" to decide what is stale, and logs from a server
	// are read next to other systems' logs: a local zone, or a daylight-saving
	// change, would only make both harder to read. Set first, before any
	// goroutine exists, because time.Local is not safe to change later.
	time.Local = time.UTC

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	// After the first signal, restore the default handling, so a second
	// Ctrl-C kills a shutdown that is taking too long instead of being ignored.
	go func() { <-ctx.Done(); stop() }()
	os.Exit(run(ctx, os.Args[1:], os.Stderr))
}

// run is main without the process: it takes its context, arguments and log
// destination, and returns the exit status, so tests can drive it.
// 0 clean exit, 1 runtime failure, 2 bad usage.
func run(ctx context.Context, args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("feedwatch", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", envOr("FEEDWATCH_LISTEN", "127.0.0.1:3102"), "loopback address to serve on (env FEEDWATCH_LISTEN)")
	shutdownTimeout := fs.Duration("shutdown-timeout", 10*time.Second, "how long in-flight requests get to finish on shutdown")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	log := newLogger(stderr)
	if err := requireLoopback(*listen); err != nil {
		log.Error("refusing to start", "err", err)
		return 2
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("listen", "err", err)
		return 1
	}
	log.Info("feedwatch listening", "addr", ln.Addr().String())

	if err := serve(ctx, ln, api.New(log, nil).Handler(), *shutdownTimeout, log); err != nil {
		log.Error("server", "err", err)
		return 1
	}
	log.Info("feedwatch stopped")
	return 0
}

// newLogger writes key=value lines with UTC timestamps. main also sets
// time.Local to UTC; this makes run's logs UTC on its own, so tests and any
// other caller get the same output.
func newLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Value = slog.TimeValue(a.Value.Time().UTC())
			}
			return a
		},
	}))
}

// serve runs the HTTP server on ln until ctx is cancelled, then shuts down
// gracefully: no new connections, in-flight requests get up to timeout to
// finish. It returns nil on a clean shutdown.
func serve(ctx context.Context, ln net.Listener, h http.Handler, timeout time.Duration, log *slog.Logger) error {
	srv := &http.Server{
		Handler: h,
		// Without these a slow or idle client can hold a connection open
		// for ever. Small numbers suit a loopback service that only reads.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}

	// Buffered, so the goroutine can always deliver its result and exit,
	// even if nobody is left to receive it.
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		// Serve only returns early on a real failure.
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down", "timeout", timeout)
	// A fresh context: ctx is already cancelled, and the deadline here is
	// for the shutdown itself.
	sctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return nil
}

// requireLoopback rejects any address that is not loopback. An empty host
// (":3102") means every interface, which is exactly what this guards against.
func requireLoopback(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("listen address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return nil
	}
	return fmt.Errorf("listen address %q is not loopback; feedwatch serves on 127.0.0.1 or ::1 only", addr)
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
