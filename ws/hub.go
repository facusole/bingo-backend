package ws

import (
	"sync"

	"github.com/facu/bingo-back/store"
)

// sendBuffer is the per-connection outbound buffer. A client whose buffer
// fills up (too slow to keep up) is dropped rather than blocking the room.
const sendBuffer = 16

// Client is one connection in a room. In the hub it has no socket: send is its
// outbound mailbox, which the write pump drains.
type Client struct {
	PlayerID store.PlayerID
	RoomID   store.RoomID
	send     chan []byte
}

// NewClient creates a client with an empty outbound buffer.
func NewClient(playerID store.PlayerID, roomID store.RoomID) *Client {
	return &Client{
		PlayerID: playerID,
		RoomID:   roomID,
		send:     make(chan []byte, sendBuffer),
	}
}

// unicast is a message addressed to a single client.
type unicast struct {
	c   *Client
	msg []byte
}

// Hub owns all the connections of a single room. It is the only goroutine that
// touches the clients map, so it needs no mutex (actor pattern).
type Hub struct {
	roomID     store.RoomID
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	sendOne    chan unicast
	sendEach   chan map[store.PlayerID][]byte
	count      chan chan int
	stop       chan struct{}
	stopOnce   sync.Once
	done       chan struct{}
}

func newHub(roomID store.RoomID) *Hub {
	return &Hub{
		roomID:     roomID,
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte),
		sendOne:    make(chan unicast),
		sendEach:   make(chan map[store.PlayerID][]byte),
		count:      make(chan chan int),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// run is the hub's event loop. Start it with `go h.run()`.
func (h *Hub) run() {
	defer close(h.done)
	for {
		select {
		case c := <-h.register:
			h.clients[c] = true
		case c := <-h.unregister:
			h.removeClient(c)
		case msg := <-h.broadcast:
			for c := range h.clients {
				h.deliver(c, msg)
			}
		case u := <-h.sendOne:
			if h.clients[u.c] {
				h.deliver(u.c, u.msg)
			}
		case byID := <-h.sendEach:
			for c := range h.clients {
				if msg, ok := byID[c.PlayerID]; ok {
					h.deliver(c, msg)
				}
			}
		case resp := <-h.count:
			resp <- len(h.clients)
		case <-h.stop:
			for c := range h.clients {
				close(c.send)
			}
			h.clients = nil
			return
		}
	}
}

// deliver pushes a message to a client, dropping the client if its buffer is
// full so a slow connection never blocks the room.
func (h *Hub) deliver(c *Client, msg []byte) {
	select {
	case c.send <- msg:
	default:
		h.removeClient(c)
	}
}

// removeClient deletes a client and closes its mailbox. Guarded by map
// membership so a client's channel is never closed twice.
func (h *Hub) removeClient(c *Client) {
	if _, ok := h.clients[c]; ok {
		delete(h.clients, c)
		close(c.send)
	}
}

// Register adds a client to the room.
func (h *Hub) Register(c *Client) {
	select {
	case h.register <- c:
	case <-h.done:
	}
}

// Unregister removes a client and closes its mailbox.
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Broadcast sends a message to every client in the room.
func (h *Hub) Broadcast(msg []byte) {
	select {
	case h.broadcast <- msg:
	case <-h.done:
	}
}

// SendTo sends a message to a single client, serialised through the hub.
func (h *Hub) SendTo(c *Client, msg []byte) {
	select {
	case h.sendOne <- unicast{c: c, msg: msg}:
	case <-h.done:
	}
}

// SendEach sends a per-player message: each connected client receives the
// message keyed by its own player id (used to deal individual cards).
func (h *Hub) SendEach(byID map[store.PlayerID][]byte) {
	select {
	case h.sendEach <- byID:
	case <-h.done:
	}
}

// Count returns the number of connected clients.
func (h *Hub) Count() int {
	resp := make(chan int)
	select {
	case h.count <- resp:
		return <-resp
	case <-h.done:
		return 0
	}
}

// Stop shuts the hub down and closes every client mailbox. Idempotent.
func (h *Hub) Stop() {
	h.stopOnce.Do(func() { close(h.stop) })
}

// HubManager is the registry of per-room hubs. Unlike a Hub, it is shared
// across requests, so it uses a mutex.
type HubManager struct {
	mu   sync.Mutex
	hubs map[store.RoomID]*Hub
}

// NewHubManager creates an empty registry.
func NewHubManager() *HubManager {
	return &HubManager{hubs: make(map[store.RoomID]*Hub)}
}

// GetOrCreate returns the room's hub, starting one if it does not exist yet.
func (m *HubManager) GetOrCreate(roomID store.RoomID) *Hub {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hubs[roomID]; ok {
		return h
	}
	h := newHub(roomID)
	m.hubs[roomID] = h
	go h.run()
	return h
}

// Get returns the room's hub if it exists.
func (m *HubManager) Get(roomID store.RoomID) (*Hub, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	h, ok := m.hubs[roomID]
	return h, ok
}

// Remove stops the room's hub and drops it from the registry.
func (m *HubManager) Remove(roomID store.RoomID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if h, ok := m.hubs[roomID]; ok {
		h.Stop()
		delete(m.hubs, roomID)
	}
}