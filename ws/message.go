package ws

import (
	"encoding/json"

	"github.com/facu/bingo-back/card"
	"github.com/facu/bingo-back/dto"
	"github.com/facu/bingo-back/store"
)

// Client -> server message types.
const (
	MsgJoin      = "join"
	MsgStart     = "start"
	MsgDraw      = "draw"
	MsgRestart   = "restart"
	MsgClose     = "close"
	MsgSetPrizes = "set_prizes"
)

// Server -> client message types.
const (
	MsgSnapshot      = "joined_snapshot"
	MsgPlayerList    = "player_list"
	MsgNumberDrawn   = "number_drawn"
	MsgLineAwarded   = "line_awarded"
	MsgBingoAwarded  = "bingo_awarded"
	MsgRoomClosed    = "room_closed"
	MsgError         = "error"
	MsgPrizesUpdated = "prizes_updated"
	MsgHostProgress  = "host_progress"
)

// Incoming is the envelope for every client -> server message.
type Incoming struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// Outgoing is the envelope for every server -> client message.
type Outgoing struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// Encode marshals a server -> client message into bytes ready to send.
func Encode(msgType string, data any) ([]byte, error) {
	return json.Marshal(Outgoing{Type: msgType, Data: data})
}

// ---- client -> server payloads ----

// JoinData is the first message a client sends. A non-empty Token means the
// client is reconnecting; otherwise it joins as a new player. The room id
// travels in the WebSocket URL query (/ws?room=<id>), not here.
type JoinData struct {
	Name  string `json:"name,omitempty"`
	Token string `json:"token,omitempty"`
}

// start, draw, restart and close carry no payload: the server authorises them
// by checking the connection's player id against the room's AdminID.

// ---- server -> client payloads ----

// SnapshotData is sent once, only to the joining/reconnecting client. Token is
// that client's own reconnection token (safe here because this is unicast).
// Card is a pointer so the admin (host) can serialize it as JSON null.
type SnapshotData struct {
	PlayerID    string           `json:"playerId"`
	Token       string           `json:"token"`
	IsAdmin     bool             `json:"isAdmin"`
	Card        *card.Card       `json:"card"`
	Drawn       []int            `json:"drawn"`
	LineAwarded bool             `json:"lineAwarded"`
	State       string           `json:"state"`
	Players     []dto.PlayerInfo `json:"players"`
	LinePrize   store.Prize      `json:"linePrize"`
	BingoPrize  store.Prize      `json:"bingoPrize"`
}

// SetPrizesData is the payload of an incoming "set_prizes" message from the
// admin. Allowed only in idle/finished state.
type SetPrizesData struct {
	Line  store.Prize `json:"line"`
	Bingo store.Prize `json:"bingo"`
}

// PrizesUpdatedData is broadcast after a successful "set_prizes" change.
type PrizesUpdatedData struct {
	Line  store.Prize `json:"line"`
	Bingo store.Prize `json:"bingo"`
}

// HostProgressData is sent ONLY to the room admin. It carries each non-admin
// player's distance to line/bingo so the host can show a tension meter.
type HostProgressData struct {
	Players []store.PlayerProgress `json:"players"`
}

// PlayerListData is broadcast whenever the roster changes.
type PlayerListData struct {
	Players []dto.PlayerInfo `json:"players"`
}

// NumberDrawnData carries only the new number; the client accumulates history.
type NumberDrawnData struct {
	Number int `json:"number"`
}

// WinnersData carries player ids; used for both line and bingo awards.
type WinnersData struct {
	Winners []string `json:"winners"`
}

// ErrorData is sent before closing a connection on a fatal error.
type ErrorData struct {
	Message string `json:"message"`
}