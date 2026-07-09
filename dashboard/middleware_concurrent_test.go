package dashboard

import (
        "sync"
        "testing"

        "github.com/nelthaarion/breeze"
)

// TestMiddleware_ConcurrentNoCrash verifies that the dashboard middleware
// does not crash under concurrent load when accessing ctx.Conn after
// ctx.Next() returns.
//
// This test reproduces the crash scenario from the stack trace:
//   clientIP(...) middleware.go:229
//   Collector.Middleware(...) middleware.go:65
//
// The root cause: the dashboard middleware called ctx.Conn.RemoteAddr()
// AFTER ctx.Next() returned. If the connection was closed concurrently
// (client disconnect, gnet fd reuse), this was a use-after-close bug.
//
// The fix: capture all needed values (IP, method, path, user-agent)
// BEFORE calling ctx.Next(), so we never touch ctx.Conn after the
// handler returns.
func TestMiddleware_ConcurrentNoCrash(t *testing.T) {
        cfg := DefaultConfig()
        cfg.Timeline = false // isolate the crash from timeline code
        c := newCollector(cfg, nil)

        mw := Middleware(c)

        // Run 100 concurrent "requests" through the middleware.
        const n = 100
        var wg sync.WaitGroup
        wg.Add(n)

        for i := 0; i < n; i++ {
                go func() {
                        defer wg.Done()
                        // Simulate a request with a nil Conn (as would happen
                        // if the connection was closed mid-request).
                        ctx := breeze.NewContext(breeze.GET, "/api/test")
                        ctx.Req.Header["user-agent"] = "test"
                        ctx.SetMiddlewareChain(nil, func(ctx *breeze.Context) {
                                ctx.WriteString("ok")
                        })
                        // This should NOT panic even though Conn is nil.
                        // (Before the fix, clientIP() would dereference ctx.Conn
                        // and crash.)
                        defer func() {
                                if r := recover(); r != nil {
                                        t.Errorf("middleware panicked: %v", r)
                                }
                        }()
                        mw(ctx)
                }()
        }

        wg.Wait()
}

// TestMiddleware_CapturesValuesBeforeNext verifies that the middleware
// captures all context-derived values BEFORE ctx.Next(), so that
// it never needs to touch ctx.Conn (or any mutable context state) after
// the handler returns.
//
// NOTE: With the sampling optimization, the middleware only captures
// full records when there are WS clients connected OR the request is
// slow/errored. This test forces capture by returning a 500 status.
func TestMiddleware_CapturesValuesBeforeNext(t *testing.T) {
        cfg := DefaultConfig()
        cfg.Timeline = false
        c := newCollector(cfg, nil)

        handlerRan := false
        mw := Middleware(c)

        ctx := breeze.NewContext(breeze.GET, "/api/users")
        ctx.Req.Header["user-agent"] = "test-agent"
        ctx.Req.Header["x-forwarded-for"] = "1.2.3.4"
        // Return 500 to force capture (errors are always captured).
        ctx.SetMiddlewareChain(nil, func(ctx *breeze.Context) {
                handlerRan = true
                ctx.Status(500)
                ctx.JSON(map[string]string{"error": "test"})
        })

        mw(ctx)

        if !handlerRan {
                t.Error("handler did not run")
        }

        // Verify the request was recorded with the correct values.
        recs := c.Requests(10)
        if len(recs) != 1 {
                t.Fatalf("expected 1 request record, got %d", len(recs))
        }
        r := recs[0]
        if r.Method != "GET" {
                t.Errorf("Method = %q, want GET", r.Method)
        }
        if r.Path != "/api/users" {
                t.Errorf("Path = %q, want /api/users", r.Path)
        }
        if r.IP != "1.2.3.4" {
                t.Errorf("IP = %q, want 1.2.3.4", r.IP)
        }
        if r.UserAgent != "test-agent" {
                t.Errorf("UserAgent = %q, want test-agent", r.UserAgent)
        }
}

// TestMiddleware_NilConnNoPanic verifies that the middleware handles a
// nil ctx.Conn gracefully (no panic). This is the direct reproduction of
// the reported crash.
func TestMiddleware_NilConnNoPanic(t *testing.T) {
        cfg := DefaultConfig()
        c := newCollector(cfg, nil)
        mw := Middleware(c)

        ctx := breeze.NewContext(breeze.GET, "/api/test")
        // No X-Forwarded-For header, so clientIP() will try ctx.Conn.RemoteAddr()
        // — but ctx.Conn is nil.
        ctx.SetMiddlewareChain(nil, func(ctx *breeze.Context) {
                ctx.WriteString("ok")
        })

        defer func() {
                if r := recover(); r != nil {
                        t.Errorf("middleware panicked with nil Conn: %v", r)
                }
        }()

        mw(ctx)
}
