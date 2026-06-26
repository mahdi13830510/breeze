package main

import (
	"fmt"
	"runtime"

	"github.com/nelthaarion/breeze"
)

// ─── HTTP request / response types ───────────────────────────────────────────

type CreateUserRequest struct {
	Name  string `json:"name"  description:"Full name of the user"`
	Email string `json:"email" description:"Email address"`
	Age   int    `json:"age"   description:"Age in years"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type UserListResponse struct {
	Users []UserResponse `json:"users"`
	Total int            `json:"total"`
}

// ─── WebSocket chat handler ───────────────────────────────────────────────────

type ChatHandler struct {
	hub *breeze.WSHub
}

func (h *ChatHandler) OnConnect(conn *breeze.WSConn) {
	fmt.Printf("[ws] client connected: %s (total: %d)\n", conn.RemoteAddr(), h.hub.Count())
	h.hub.BroadcastExcept(breeze.WsOpText, []byte("a user joined"), conn)
}

func (h *ChatHandler) OnMessage(conn *breeze.WSConn, opcode byte, payload []byte) {
	if opcode == breeze.WsOpText {
		msg := fmt.Sprintf("[%s]: %s", conn.RemoteAddr(), string(payload))
		h.hub.BroadcastText(msg)
	} else {
		_ = conn.SendBinary(payload)
	}
}

func (h *ChatHandler) OnClose(conn *breeze.WSConn, code uint16, reason string) {
	fmt.Printf("[ws] client disconnected: %s code=%d reason=%q (remaining: %d)\n",
		conn.RemoteAddr(), code, reason, h.hub.Count())
	h.hub.BroadcastText("a user left")
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	router := breeze.NewRouter()
	pool := breeze.NewWorkerPool(runtime.NumCPU())
	app := breeze.New(router, pool)

	// WebSocket() returns the hub immediately — inject it into the handler.
	chat := &ChatHandler{}
	chat.hub = app.WebSocket("/ws", chat)

	// Inline echo endpoint using WSHandlerFunc.
	app.WebSocket("/ws/echo", &breeze.WSHandlerFunc{
		Connect: func(conn *breeze.WSConn) {

			_ = conn.SendText("echo server ready")
		},
		Message: func(conn *breeze.WSConn, opcode byte, payload []byte) {
			_ = conn.Send(opcode, payload)
		},
	})

	// ── HTTP routes ───────────────────────────────────────────────────────

	router.Handle(breeze.GET, "/users", listUsers)
	router.Handle(breeze.POST, "/users", createUser)
	router.Handle(breeze.GET, "/users/:id", getUser)
	router.Handle(breeze.DELETE, "/users/:id", deleteUser)

	router.Handle(breeze.GET, "/ws/stats", func(ctx *breeze.Context) {
		count := int64(0)
		if h := app.Hub(); h != nil {
			count = h.Count()
		}
		ctx.JSON(map[string]int64{"connections": count})
	})

	router.Handle(breeze.GET, "/", func(ctx *breeze.Context) {
		ctx.WriteString("Breeze — HTTP + WebSocket server")
	})

	fmt.Println("Breeze listening on :3000")
	fmt.Println("  WebSocket chat : ws://localhost:3000/ws")
	fmt.Println("  WebSocket echo : ws://localhost:3000/ws/echo")
	fmt.Println("  WS stats       : GET /ws/stats")
	app.Run(3000, true)
}

// ─── HTTP handlers ────────────────────────────────────────────────────────────

func listUsers(ctx *breeze.Context) {
	ctx.JSON(UserListResponse{
		Users: []UserResponse{{ID: "1", Name: "Alice", Email: "alice@example.com"}},
		Total: 1,
	})
}

func createUser(ctx *breeze.Context) {
	ctx.Status(201)
	ctx.JSON(UserResponse{ID: "2", Name: "Bob", Email: "bob@example.com"})
}

func getUser(ctx *breeze.Context) {
	ctx.JSON(UserResponse{
		ID:    ctx.GetParam("id"),
		Name:  "Alice",
		Email: "alice@example.com",
	})
}

func deleteUser(ctx *breeze.Context) {
	ctx.Status(204)
}
