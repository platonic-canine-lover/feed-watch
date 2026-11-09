package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestRequireLoopback(t *testing.T) {
	tests := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:3102", true},
		{"127.0.0.53:3102", true}, // all of 127.0.0.0/8 is loopback
		{"[::1]:3102", true},
		{"localhost:3102", true},
		{":3102", false}, // every interface
		{"0.0.0.0:3102", false},
		{"[::]:3102", false},
		{"192.168.8.190:3102", false},
		{"example.com:3102", false},
		{"127.0.0.1", false}, // no port
	}
	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			err := requireLoopback(tt.addr)
			if (err == nil) != tt.ok {
				t.Fatalf("requireLoopback(%q) = %v, want ok=%v", tt.addr, err, tt.ok)
			}
		})
	}
}

func TestRunRefusesBadUsage(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown flag", []string{"-nope"}, "flag provided but not defined"},
		{"not loopback", []string{"-listen", "0.0.0.0:0"}, "not loopback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if code := run(context.Background(), tt.args, &out); code != 2 {
				t.Fatalf("exit = %d, want 2", code)
			}
			if !strings.Contains(out.String(), tt.want) {
				t.Errorf("output %q does not mention %q", out.String(), tt.want)
			}
		})
	}
}

// The shutdown contract: a request already in flight when the signal
// arrives is allowed to finish, and serve then returns nil.
func TestServeFinishesInFlightRequestsOnShutdown(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inside := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inside)
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte("finished"))
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, h, 5*time.Second, quiet()) }()

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + ln.Addr().String() + "/")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		got <- result{string(b), err}
	}()

	<-inside // the request is being handled
	cancel() // the "signal"

	r := <-got
	if r.err != nil || r.body != "finished" {
		t.Fatalf("in-flight request: body %q, err %v", r.body, r.err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return after shutdown")
	}
	if _, err := net.DialTimeout("tcp", ln.Addr().String(), time.Second); err == nil {
		t.Error("still accepting connections after shutdown")
	}
}

// A request that outlives the shutdown timeout makes serve report it,
// rather than hang or claim a clean stop.
func TestServeReportsAShutdownThatRunsOut(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	inside := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inside)
		<-release
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, h, 50*time.Millisecond, quiet()) }()
	go func() {
		if resp, err := http.Get("http://" + ln.Addr().String() + "/"); err == nil {
			resp.Body.Close()
		}
	}()

	<-inside
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "shutdown") {
			t.Fatalf("serve returned %v, want a shutdown error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve hung past its shutdown timeout")
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }
