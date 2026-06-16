package ws

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/facu/bingo-back/card"
	"github.com/facu/bingo-back/store"
	"github.com/gorilla/websocket"
)

func drainSnapshotAndList(t *testing.T, c *websocket.Conn) {
	t.Helper()
	if e := readEnv(t, c); e.Type != MsgSnapshot {
		t.Fatalf("expected snapshot, got %q", e.Type)
	}
	if e := readEnv(t, c); e.Type != MsgPlayerList {
		t.Fatalf("expected player_list, got %q", e.Type)
	}
}

func sendAction(t *testing.T, c *websocket.Conn, msgType string) {
	t.Helper()
	env, _ := json.Marshal(Incoming{Type: msgType})
	if err := c.WriteMessage(websocket.TextMessage, env); err != nil {
		t.Fatalf("write %s: %v", msgType, err)
	}
}

func TestStartDealsCardsToEveryone(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	bob := joinWS(t, wsURL, room.ID, "bob", "")
	defer bob.Close()
	drainSnapshotAndList(t, bob)
	// admin learns bob joined
	if e := readEnv(t, adminConn); e.Type != MsgPlayerList {
		t.Fatalf("admin expected player_list, got %q", e.Type)
	}

	sendAction(t, adminConn, MsgStart)

	for name, c := range map[string]*websocket.Conn{"admin": adminConn, "bob": bob} {
		e := readEnv(t, c)
		if e.Type != MsgSnapshot {
			t.Fatalf("%s after start got %q, want snapshot", name, e.Type)
		}
		var snap SnapshotData
		json.Unmarshal(e.Data, &snap)
		if snap.State != string(store.StateActive) {
			t.Fatalf("%s state = %q, want active", name, snap.State)
		}
		if snap.Card == (card.Card{}) {
			t.Fatalf("%s got an empty card after start", name)
		}
	}
}

func TestDrawBroadcastsNumber(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	sendAction(t, adminConn, MsgStart)
	if e := readEnv(t, adminConn); e.Type != MsgSnapshot {
		t.Fatalf("expected snapshot after start, got %q", e.Type)
	}

	sendAction(t, adminConn, MsgDraw)
	e := readEnv(t, adminConn)
	if e.Type != MsgNumberDrawn {
		t.Fatalf("after draw got %q, want number_drawn", e.Type)
	}
	var nd NumberDrawnData
	json.Unmarshal(e.Data, &nd)
	if nd.Number < 1 || nd.Number > 90 {
		t.Fatalf("drawn number %d out of range", nd.Number)
	}
}

func TestNonAdminCannotDraw(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	bob := joinWS(t, wsURL, room.ID, "bob", "")
	defer bob.Close()
	drainSnapshotAndList(t, bob)
	readEnv(t, adminConn) // admin's player_list for bob

	sendAction(t, bob, MsgDraw) // non-admin
	if e := readEnv(t, bob); e.Type != MsgError {
		t.Fatalf("non-admin draw got %q, want error", e.Type)
	}
}

func TestCloseRoomRemovesIt(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	sendAction(t, adminConn, MsgClose)
	if e := readEnv(t, adminConn); e.Type != MsgRoomClosed {
		t.Fatalf("after close got %q, want room_closed", e.Type)
	}

	// the room is removed from the store (the action is async, so poll briefly)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := st.GetRoom(room.ID); err != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("room was not removed from the store")
		}
		time.Sleep(10 * time.Millisecond)
	}
}