package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestPumpsEndToEnd spins up a real WebSocket and checks both directions:
// a client message reaches onMessage (readPump), and a hub broadcast reaches
// the client over the socket (writePump).
func TestPumpsEndToEnd(t *testing.T) {
	hub := newHub("r1")
	go hub.run()
	defer hub.Stop()

	received := make(chan Incoming, 1)

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		client := NewClient("p1", "r1")
		hub.Register(client)
		Serve(client, conn, hub, func(_ *Client, msg Incoming) {
			received <- msg
		}, nil)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	// client -> server: readPump parses and dispatches
	if err := c.WriteMessage(websocket.TextMessage, []byte(`{"type":"draw"}`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case msg := <-received:
		if msg.Type != MsgDraw {
			t.Fatalf("server got type %q, want %q", msg.Type, MsgDraw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server never received the client message")
	}

	// server -> client: a hub broadcast reaches the socket via writePump
	hub.Broadcast([]byte(`{"type":"number_drawn"}`))
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read broadcast: %v", err)
	}
	var out Incoming
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("unmarshal broadcast: %v", err)
	}
	if out.Type != MsgNumberDrawn {
		t.Fatalf("client got type %q, want %q", out.Type, MsgNumberDrawn)
	}
}