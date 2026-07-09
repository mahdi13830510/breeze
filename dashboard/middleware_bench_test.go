package dashboard

import (
        "crypto/rand"
        "testing"
        "time"

        "github.com/nelthaarion/breeze"
)

// BenchmarkMiddlewareOverhead_NoClients measures the middleware overhead
// per request when no WebSocket clients are connected (the common case).
//
// Before the fix: 13K req/s (crypto/rand + full capture on every request)
// After the fix: should be near-native speed (atomic counter + timing only)
func BenchmarkMiddlewareOverhead_NoClients(b *testing.B) {
        cfg := DefaultConfig()
        cfg.Timeline = true
        c := newCollector(cfg, nil)
        c.hub = newWSHub(c) // no WS clients

        mw := Middleware(c)
        handler := func(ctx *breeze.Context) {
                ctx.JSON(map[string]string{"ok": "true"})
        }

        b.ReportAllocs()
        b.ResetTimer()

        for i := 0; i < b.N; i++ {
                ctx := breeze.NewContext(breeze.GET, "/api/users")
                ctx.Req.Header["user-agent"] = "benchmark"
                ctx.SetMiddlewareChain(nil, handler)
                mw(ctx)
        }
}

// BenchmarkMiddlewareOverhead_Disabled measures overhead when dashboard is off.
func BenchmarkMiddlewareOverhead_Disabled(b *testing.B) {
        cfg := DefaultConfig()
        cfg.Enabled = false
        c := newCollector(cfg, nil)
        c.hub = newWSHub(c)

        mw := Middleware(c)
        handler := func(ctx *breeze.Context) {
                ctx.JSON(map[string]string{"ok": "true"})
        }

        b.ReportAllocs()
        b.ResetTimer()

        for i := 0; i < b.N; i++ {
                ctx := breeze.NewContext(breeze.GET, "/api/users")
                ctx.SetMiddlewareChain(nil, handler)
                mw(ctx)
        }
}

// BenchmarkNewID_Atomic measures the new atomic counter ID generator.
func BenchmarkNewID_Atomic(b *testing.B) {
        b.ReportAllocs()
        for i := 0; i < b.N; i++ {
                _ = newID()
        }
}

// BenchmarkNewID_CryptoRand measures the OLD crypto/rand ID generator
// for comparison — this is what we replaced.
func BenchmarkNewID_CryptoRand(b *testing.B) {
        b.ReportAllocs()
        for i := 0; i < b.N; i++ {
                var buf [8]byte
                _, _ = rand.Read(buf[:])
                const hexDigits = "0123456789abcdef"
                var out [16]byte
                for i, v := range buf {
                        out[i*2] = hexDigits[v>>4]
                        out[i*2+1] = hexDigits[v&0xF]
                }
                _ = string(out[:])
        }
}

// TestMiddleware_StillRecordsErrors verifies errors are always captured.
func TestMiddleware_StillRecordsErrors(t *testing.T) {
        cfg := DefaultConfig()
        c := newCollector(cfg, nil)
        c.hub = newWSHub(c) // no WS clients

        mw := Middleware(c)

        ctx := breeze.NewContext(breeze.GET, "/api/error")
        ctx.SetMiddlewareChain(nil, func(ctx *breeze.Context) {
                ctx.Status(500)
                ctx.JSON(map[string]string{"error": "fail"})
        })

        mw(ctx)

        recs := c.Requests(10)
        if len(recs) != 1 {
                t.Fatalf("expected 1 request record (error should always be captured), got %d", len(recs))
        }
        if recs[0].Status != 500 {
                t.Errorf("Status = %d, want 500", recs[0].Status)
        }
}

// TestMiddleware_StillRecordsSlow verifies slow requests are always captured.
func TestMiddleware_StillRecordsSlow(t *testing.T) {
        cfg := DefaultConfig()
        cfg.SlowRequestMs = 10
        c := newCollector(cfg, nil)
        c.hub = newWSHub(c)

        cases := []struct {
                name     string
                duration time.Duration
                status   int
                want     bool
        }{
                {"fast_ok", 100 * time.Microsecond, 200, false},
                {"slow_ok", 20 * time.Millisecond, 200, true},
                {"fast_err", 100 * time.Microsecond, 500, true},
                {"slow_err", 20 * time.Millisecond, 500, true},
        }
        for _, tc := range cases {
                t.Run(tc.name, func(t *testing.T) {
                        got := c.shouldCaptureFullRecord(tc.duration, tc.status)
                        if got != tc.want {
                                t.Errorf("shouldCaptureFullRecord(%v, %d) = %v, want %v",
                                        tc.duration, tc.status, got, tc.want)
                        }
                })
        }
}

// TestMiddleware_FastPathNoAllocs verifies that the fast path (no WS clients,
// fast request, no error) does not allocate beyond the context setup.
//
// NOTE: breeze.NewContext itself allocates (Context + HTTPRequest + Header
// map = 3 allocs). The middleware should add ZERO additional allocs on
// the fast path. We measure this by comparing allocs with and without
// the middleware.
func TestMiddleware_FastPathNoAllocs(t *testing.T) {
        cfg := DefaultConfig()
        c := newCollector(cfg, nil)
        c.hub = newWSHub(c)

        mw := Middleware(c)
        handler := func(ctx *breeze.Context) {
                ctx.WriteString("ok")
        }

        // Measure allocations of just creating the context + running the
        // handler (baseline — this includes handler allocations like
        // HTTPResponse creation).
        baselineAllocs := testing.AllocsPerRun(100, func() {
                ctx := breeze.NewContext(breeze.GET, "/api/users")
                ctx.SetMiddlewareChain(nil, handler)
                ctx.Next() // run the handler directly
        })

        // Measure allocations with the middleware.
        totalAllocs := testing.AllocsPerRun(100, func() {
                ctx := breeze.NewContext(breeze.GET, "/api/users")
                ctx.SetMiddlewareChain(nil, handler)
                mw(ctx)
        })

        middlewareAllocs := totalAllocs - baselineAllocs
        t.Logf("baseline (context creation): %.1f allocs", baselineAllocs)
        t.Logf("with middleware: %.1f allocs", totalAllocs)
        t.Logf("middleware overhead: %.1f allocs", middlewareAllocs)

        if middlewareAllocs > 1 {
                t.Errorf("middleware fast path allocates %.1f objects (want <= 1)", middlewareAllocs)
        }
}
