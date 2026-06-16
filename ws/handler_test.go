package ws

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/facu/bingo-back/store"
	"github.com/gorilla/websocket"
)

type envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

func newWSServer(t *testing.T) (*store.Store, string, func()) {
	t.Helper()
	st := store.NewStore()
	h := &Handler{Store: st, Hubs: NewHubManager()}
	srv := httptest.NewServer(http.HandlerFunc(h.ServeWS))
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	return st, wsURL, srv.Close
}

func joinWS(t *testing.T, wsURL string, roomID store.RoomID, name, token string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(wsURL+"?room="+string(roomID), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	payload, _ := json.Marshal(JoinData{Name: name, Token: token})
	env, _ := json.Marshal(Incoming{Type: MsgJoin, Data: payload})
	if err := c.WriteMessage(websocket.TextMessage, env); err != nil {
		t.Fatalf("write join: %v", err)
	}
	return c
}

func readEnv(t *testing.T, c *websocket.Conn) envelope {
	t.Helper()
	c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var e envelope
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return e
}

func TestJoinNewPlayerReceivesSnapshot(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()

	room, _, _ := st.CreateRoom("admin")
	c := joinWS(t, wsURL, room.ID, "bob", "")
	defer c.Close()

	e := readEnv(t, c)
	if e.Type != MsgSnapshot {
		t.Fatalf("first message = %q, want %q", e.Type, MsgSnapshot)
	}
	var snap SnapshotData
	if err := json.Unmarshal(e.Data, &snap); err != nil {
		t.Fatalf("snapshot decode: %v", err)
	}
	if snap.PlayerID == "" {
		t.Fatal("snapshot has empty playerId")
	}
	if snap.Token == "" {
		t.Fatal("snapshot has empty token; player could not reconnect")
	}
	if snap.State != string(store.StateIdle) {
		t.Fatalf("state = %q, want idle", snap.State)
	}
	if len(snap.Players) != 2 { // admin (from CreateRoom) + bob
		t.Fatalf("players = %d, want 2", len(snap.Players))
	}
}

func TestSecondPlayerTriggersPlayerList(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()

	room, _, _ := st.CreateRoom("admin")

	a := joinWS(t, wsURL, room.ID, "a", "")
	defer a.Close()
	if e := readEnv(t, a); e.Type != MsgSnapshot {
		t.Fatalf("a first = %q, want snapshot", e.Type)
	}
	if e := readEnv(t, a); e.Type != MsgPlayerList {
		t.Fatalf("a second = %q, want player_list", e.Type)
	}

	b := joinWS(t, wsURL, room.ID, "b", "")
	defer b.Close()

	e := readEnv(t, a) // a learns that b joined
	if e.Type != MsgPlayerList {
		t.Fatalf("a after b joined = %q, want player_list", e.Type)
	}
	var pl PlayerListData
	json.Unmarshal(e.Data, &pl)
	if len(pl.Players) != 3 { // admin + a + b
		t.Fatalf("players = %d, want 3", len(pl.Players))
	}
}

func TestReconnectKeepsPlayerID(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()

	room, _, _ := st.CreateRoom("admin")

	a := joinWS(t, wsURL, room.ID, "bob", "")
	var s1 SnapshotData
	json.Unmarshal(readEnv(t, a).Data, &s1)
	a.Close()

	r := joinWS(t, wsURL, room.ID, "", s1.Token)
	defer r.Close()
	var s2 SnapshotData
	json.Unmarshal(readEnv(t, r).Data, &s2)

	if s2.PlayerID != s1.PlayerID {
		t.Fatalf("reconnect playerId = %q, want %q", s2.PlayerID, s1.PlayerID)
	}
}

func TestRoomNotFound(t *testing.T) {
	_, wsURL, closeFn := newWSServer(t)
	defer closeFn()

	_, resp, err := websocket.DefaultDialer.Dial(wsURL+"?room=nope", nil)
	if err == nil {
		t.Fatal("expected dial to fail for a missing room")
	}
	if resp == nil || resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %v, want 404", resp)
	}
}

func TestFirstMessageMustBeJoin(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()

	room, _, _ := st.CreateRoom("admin")
	c, _, err := websocket.DefaultDialer.Dial(wsURL+"?room="+string(room.ID), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.Close()

	c.WriteMessage(websocket.TextMessage, []byte(`{"type":"draw"}`)) // not a join

	if e := readEnv(t, c); e.Type != MsgError {
		t.Fatalf("type = %q, want error", e.Type)
	}
}