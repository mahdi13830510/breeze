package breeze

import (
	"fmt"
	"sync"

	"github.com/panjf2000/gnet/v2"
)

type Breeze struct {
	*gnet.BuiltinEventEngine
	Router *Router
	bufs   sync.Map // fd(int) → []byte ; per-connection reassembly buffer
	Pool   *WorkerPool
}

// compactThreshold: compact the leftover slice when the unused capacity
// exceeds this many bytes, to avoid keeping large receive buffers alive.
const compactThreshold = 512

func New(router *Router, pool *WorkerPool) *Breeze {
	return &Breeze{
		BuiltinEventEngine: &gnet.BuiltinEventEngine{},
		Router:             router,
		Pool:               pool,
	}
}

func (s *Breeze) OnTraffic(c gnet.Conn) gnet.Action {
	fd := c.Fd()
	data, _ := c.Next(-1)
	if len(data) == 0 {
		return gnet.None
	}

	// Always copy incoming data into a Go-owned buffer.
	//
	// gnet's data slice is a view into gnet's internal ring buffer.
	// gnet is free to overwrite that memory as soon as OnTraffic returns,
	// but req.Body may still reference it from a worker goroutine.
	// By appending into an existing Go slice (or a fresh one when existing
	// is nil), we ensure buf is always GC-managed: req.Body = buf[x:y]
	// keeps the backing array alive for exactly as long as the request lives.
	//
	// Cost: one append per OnTraffic call (not per request). On subsequent
	// reads for the same fd the append grows the existing slice in-place when
	// there is enough capacity, so the amortized cost is low.
	var existing []byte
	if v, ok := s.bufs.Load(fd); ok {
		existing = v.([]byte)
	}
	buf := append(existing, data...)

	for len(buf) > 0 {
		req, consumed, err := ParseHTTPRequest(buf)
		if err != nil {
			c.AsyncWrite([]byte("HTTP/1.1 400 Bad Request\r\nContent-Length: 11\r\n\r\nBad Request"), nil)
			buf = nil
			break
		}
		if req == nil {
			break // incomplete — wait for more data
		}

		handler, middlewares, params := s.Router.Find(req)

		if handler == nil {
			c.AsyncWrite([]byte("HTTP/1.1 404 Not Found\r\nContent-Length: 9\r\n\r\nNot Found"), nil)
		} else {
			ctx := &Context{
				Conn:        c,
				Req:         req,
				params:      params,
				middlewares: append(middlewares, handler),
				index:       -1,
			}

			exec := func() {
				ctx.Next()
				if ctx.Res != nil {
					c.AsyncWrite(ctx.Res.Bytes(), nil)
				}
			}

			if s.Pool != nil {
				s.Pool.Submit(exec)
			} else {
				go exec()
			}
		}

		if consumed >= len(buf) {
			buf = nil
			break
		}
		buf = buf[consumed:]
	}

	// Store leftover bytes (partial next request).
	if len(buf) == 0 {
		s.bufs.Delete(fd)
	} else {
		// Compact: if the unused capacity behind the leftover slice is large,
		// copy to a fresh allocation so the old backing array can be GC'd.
		// Note: req.Body for completed requests holds its own reference to the
		// old array; the GC will not collect it until those requests are done.
		if cap(buf)-len(buf) > compactThreshold {
			compact := make([]byte, len(buf))
			copy(compact, buf)
			buf = compact
		}
		s.bufs.Store(fd, buf)
	}

	return gnet.None
}

// OnClose cleans up the per-connection buffer when a connection closes.
func (s *Breeze) OnClose(c gnet.Conn, err error) gnet.Action {
	s.bufs.Delete(c.Fd())
	return gnet.None
}

func (s *Breeze) Run(port int, multiCore bool) error {
	return gnet.Run(
		s,
		fmt.Sprintf("tcp://:%d", port),
		gnet.WithTCPNoDelay(gnet.TCPNoDelay),
		gnet.WithMulticore(multiCore),
		gnet.WithLoadBalancing(gnet.RoundRobin),
	)
}
