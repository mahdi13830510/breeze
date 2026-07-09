package dashboard

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/nelthaarion/breeze"
)

// wsHub multiplexes dashboard WebSocket connections and broadcasts periodic
// snapshots and live events to every connected client.
//
// Two kinds of messages are sent:
//
//   1. "snapshot" — a full state snapshot, sent on connect and every second.
//   2. "event"    — a single live record (request / query / log / timeline),
//      pushed the moment it is recorded by a collector.
//
// The hub is intentionally lightweight: it does not buffer messages. A slow
// client will drop messages (we never block the hot path). This is acceptable
// for a developer dashboard.
type wsHub struct {
	mu      sync.RWMutex
	clients map[*breeze.WSConn]struct{}
	c       *Collector
	stop    chan struct{}
}

func newWSHub(c *Collector) *wsHub {
	h := &wsHub{
		clients: make(map[*breeze.WSConn]struct{}),
		c:       c,
		stop:    make(chan struct{}),
	}
	go h.broadcastLoop()
	return h
}

// register adds a client and immediately sends a snapshot.
func (h *wsHub) register(conn *breeze.WSConn) {
	h.mu.Lock()
	h.clients[conn] = struct{}{}
	h.mu.Unlock()
	_ = conn.SendText(h.snapshotMessage())
}

// unregister removes a client.
func (h *wsHub) unregister(conn *breeze.WSConn) {
	h.mu.Lock()
	delete(h.clients, conn)
	h.mu.Unlock()
}

// broadcast sends a text message to every connected client.
func (h *wsHub) broadcast(msg string) {
	h.mu.RLock()
	targets := make([]*breeze.WSConn, 0, len(h.clients))
	for c := range h.clients {
		targets = append(targets, c)
	}
	h.mu.RUnlock()
	for _, c := range targets {
		_ = c.SendText(msg)
	}
}

// broadcastLoop sends a full snapshot every second so the overview page's
// charts keep ticking even when no events arrive.
func (h *wsHub) broadcastLoop() {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.broadcast(h.snapshotMessage())
		}
	}
}

// snapshotMessage builds a JSON envelope containing the dashboard overview.
func (h *wsHub) snapshotMessage() string {
	m := map[string]any{
		"type":    "snapshot",
		"time":     time.Now().UTC().Format(time.RFC3339),
		"metrics":  h.c.Metrics(),
		"routes":   h.c.RouteStats(),
		"queue":    h.c.QueueStats(),
		"cache":    h.c.CacheStats(),
	}
	b, _ := json.Marshal(m)
	return string(b)
}

// pushEvent broadcasts a single live event to all connected clients.
// Called from the hot path (request, query, log, timeline recorders).
func (h *wsHub) pushEvent(kind string, payload any) {
	if h.clientCount() == 0 {
		return // no clients → no work
	}
	msg, err := json.Marshal(map[string]any{
		"type":    "event",
		"channel": kind,
		"time":     time.Now().UTC().Format(time.RFC3339),
		"data":    payload,
	})
	if err != nil {
		return
	}
	h.broadcast(string(msg))
}

func (h *wsHub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ─── Bridge to Breeze WSHandler ───────────────────────────────────────────

// wsHandler adapts the dashboard hub to Breeze's WSHandler interface.
type wsHandler struct {
	hub *wsHub
}

func (h *wsHandler) OnConnect(conn *breeze.WSConn) {
	h.hub.register(conn)
}

func (h *wsHandler) OnMessage(conn *breeze.WSConn, opcode byte, payload []byte) {
	// Clients may send "ping" text frames; ignore everything else.
}

func (h *wsHandler) OnClose(conn *breeze.WSConn, code uint16, reason string) {
	h.hub.unregister(conn)
}
