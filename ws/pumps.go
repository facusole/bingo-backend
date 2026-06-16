package ws

import (
	"encoding/json"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second    // max time to write a message to the socket
	pongWait       = 60 * time.Second    // max time without a pong before the read fails
	pingPeriod     = (pongWait * 9) / 10 // ping a bit more often than pongWait
	maxMessageSize = 4096                // incoming messages are tiny (join/admin actions)
)

// connection binds a logical Client to a real WebSocket and runs its pumps.
// Keeping the socket here (not on Client) leaves hub.go free of any gorilla
// dependency: the hub only ever deals with the logical *Client.
type connection struct {
	*Client
	conn      *websocket.Conn
	hub       *Hub
	onMessage func(*Client, Incoming)
	onClose   func(*Client)
}

// Serve attaches a WebSocket to an already-registered client and starts its
// read/write pumps. onMessage handles each incoming message; onClose runs once
// when the connection ends (e.g. to mark the player disconnected). It returns
// immediately; the pumps run in their own goroutines.
func Serve(client *Client, conn *websocket.Conn, hub *Hub,
	onMessage func(*Client, Incoming), onClose func(*Client)) {
	cn := &connection{
		Client:    client,
		conn:      conn,
		hub:       hub,
		onMessage: onMessage,
		onClose:   onClose,
	}
	go cn.writePump()
	go cn.readPump()
}

// writePump is the only goroutine that writes to the socket. It drains the
// client's mailbox and sends periodic pings to keep the connection alive.
func (cn *connection) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		cn.conn.Close()
	}()
	for {
		select {
		case msg, ok := <-cn.send:
			cn.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// the hub closed the mailbox: tell the client and stop
				cn.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := cn.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			cn.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := cn.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump is the only goroutine that reads from the socket. It parses each
// message and dispatches it. When it returns (the connection died), it
// unregisters the client and runs the onClose hook.
func (cn *connection) readPump() {
	defer func() {
		cn.hub.Unregister(cn.Client)
		if cn.onClose != nil {
			cn.onClose(cn.Client)
		}
		cn.conn.Close()
	}()

	cn.conn.SetReadLimit(maxMessageSize)
	cn.conn.SetReadDeadline(time.Now().Add(pongWait))
	cn.conn.SetPongHandler(func(string) error {
		cn.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, data, err := cn.conn.ReadMessage()
		if err != nil {
			return
		}
		var msg Incoming
		if err := json.Unmarshal(data, &msg); err != nil {
			continue // ignore malformed frames
		}
		if cn.onMessage != nil {
			cn.onMessage(cn.Client, msg)
		}
	}
}