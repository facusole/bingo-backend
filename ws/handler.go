package ws

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/facu/bingo-back/config"
	"github.com/facu/bingo-back/store"
	"github.com/gorilla/websocket"
)

// joinWait is how long we wait for the first (join) message after the upgrade.
const joinWait = 10 * time.Second

// Handler serves the WebSocket endpoint. It is injected with the domain store
// and the per-room hub registry, like the HTTP API.
type Handler struct {
	Store *store.Store
	Hubs  *HubManager
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		// allow non-browser clients (no Origin) and any configured origin
		return origin == "" || config.IsAllowedOrigin(origin)
	},
}

// ServeWS upgrades the connection, runs the join handshake and wires the client
// into its room's hub.
func (h *Handler) ServeWS(w http.ResponseWriter, r *http.Request) {
	param := r.URL.Query().Get("room")
	if param == "" {
		http.Error(w, "missing room", http.StatusBadRequest)
		return
	}
	// Try the short-code alias first; if not registered, fall back to a direct
	// roomID lookup. The lookup decides — never the length of the param.
	room, err := h.Store.RoomByShortCode(param)
	if err != nil {
		room, err = h.Store.GetRoom(store.RoomID(param))
		if err != nil {
			http.Error(w, "room not found", http.StatusNotFound)
			return
		}
	}
	roomID := room.ID

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // upgrader already wrote a response
	}

	// After the upgrade we can only report errors as WS messages, not HTTP.
	player, err := h.join(conn, room)
	if err != nil {
		writeError(conn, err.Error())
		conn.Close()
		return
	}

	hub := h.Hubs.GetOrCreate(roomID)
	client := NewClient(player.ID, roomID)
	hub.Register(client)

	sess := &session{handler: h, room: room, hub: hub, client: client}
	Serve(client, conn, hub,
		func(c *Client, msg Incoming) { sess.handle(msg) },
		func(c *Client) {
			room.MarkDisconnected(c.PlayerID)
			broadcastPlayerList(hub, room)
		},
	)

	// snapshot only to the joining client; roster update to everyone
	sendSnapshot(hub, client, room, player)
	broadcastPlayerList(hub, room)
}

// join reads the first message (which must be a join) and authenticates the
// player against the store: with a token it reconnects, otherwise it joins anew.
func (h *Handler) join(conn *websocket.Conn, room *store.Room) (*store.Player, error) {
	conn.SetReadDeadline(time.Now().Add(joinWait))
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, errors.New("no join message received")
	}
	conn.SetReadDeadline(time.Time{}) // clear; the read pump sets its own

	var in Incoming
	if err := json.Unmarshal(data, &in); err != nil || in.Type != MsgJoin {
		return nil, errors.New("first message must be 'join'")
	}
	var jd JoinData
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &jd); err != nil {
			return nil, errors.New("invalid join payload")
		}
	}

	if jd.Token != "" {
		p, err := h.Store.Reconnect(room.ID, store.Token(jd.Token))
		if err != nil {
			return nil, errors.New("reconnect failed")
		}
		return p, nil
	}
	return h.Store.AddPlayer(room.ID, jd.Name)
}

func sendSnapshot(hub *Hub, c *Client, room *store.Room, player *store.Player) {
	snap := SnapshotData{
		PlayerID:    string(player.ID),
		Token:       string(player.Token),
		IsAdmin:     player.ID == room.AdminID,
		Card:        room.PlayerCard(player.ID),
		Drawn:       room.Drawn(),
		LineAwarded: room.LineAwarded(),
		State:       string(room.State()),
		Players:     PlayersToDTO(room.SnapshotPlayers(), room.AdminID),
	}
	if msg, err := Encode(MsgSnapshot, snap); err == nil {
		hub.SendTo(c, msg)
	}
}

func broadcastPlayerList(hub *Hub, room *store.Room) {
	data := PlayerListData{Players: PlayersToDTO(room.SnapshotPlayers(), room.AdminID)}
	if msg, err := Encode(MsgPlayerList, data); err == nil {
		hub.Broadcast(msg)
	}
}

// writeError writes an error frame directly to the socket (used before the
// pumps are running, during a failed handshake).
func writeError(conn *websocket.Conn, message string) {
	if msg, err := Encode(MsgError, ErrorData{Message: message}); err == nil {
		conn.SetWriteDeadline(time.Now().Add(writeWait))
		conn.WriteMessage(websocket.TextMessage, msg)
	}
}