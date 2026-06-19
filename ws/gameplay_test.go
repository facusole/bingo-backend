package ws

import (
	"encoding/json"
	"testing"
	"time"

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

// drainHostProgress reads and asserts a single host_progress envelope. Use it
// in admin-side tests right after start/restart/draw to consume the meter
// refresh before checking the next expected event.
func drainHostProgress(t *testing.T, c *websocket.Conn) {
	t.Helper()
	if e := readEnv(t, c); e.Type != MsgHostProgress {
		t.Fatalf("expected host_progress, got %q", e.Type)
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
		if name == "admin" {
			if snap.Card != nil {
				t.Fatalf("admin should receive card=null, got %+v", *snap.Card)
			}
			continue
		}
		if snap.Card == nil {
			t.Fatalf("%s got nil card after start", name)
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
	drainHostProgress(t, adminConn) // admin gets the meter refresh after start

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

func sendActionWithData(t *testing.T, c *websocket.Conn, msgType string, data any) {
	t.Helper()
	raw, _ := json.Marshal(data)
	env, _ := json.Marshal(Incoming{Type: msgType, Data: raw})
	if err := c.WriteMessage(websocket.TextMessage, env); err != nil {
		t.Fatalf("write %s: %v", msgType, err)
	}
}

func TestSetPrizesBroadcastsAndSnapshot(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	bob := joinWS(t, wsURL, room.ID, "bob", "")
	defer bob.Close()
	drainSnapshotAndList(t, bob)
	if e := readEnv(t, adminConn); e.Type != MsgPlayerList {
		t.Fatalf("admin expected player_list, got %q", e.Type)
	}

	line := store.Prize{Enabled: true, Name: "Vino"}
	bingo := store.Prize{Enabled: true, Name: "$5000"}
	sendActionWithData(t, adminConn, MsgSetPrizes, SetPrizesData{Line: line, Bingo: bingo})

	for name, c := range map[string]*websocket.Conn{"admin": adminConn, "bob": bob} {
		e := readEnv(t, c)
		if e.Type != MsgPrizesUpdated {
			t.Fatalf("%s expected prizes_updated, got %q", name, e.Type)
		}
		var pu PrizesUpdatedData
		json.Unmarshal(e.Data, &pu)
		if pu.Line != line || pu.Bingo != bingo {
			t.Fatalf("%s prizes mismatch: %+v / %+v", name, pu.Line, pu.Bingo)
		}
	}

	// A late joiner sees the prizes in its snapshot.
	charlie := joinWS(t, wsURL, room.ID, "charlie", "")
	defer charlie.Close()
	e := readEnv(t, charlie)
	if e.Type != MsgSnapshot {
		t.Fatalf("charlie first = %q, want snapshot", e.Type)
	}
	var snap SnapshotData
	json.Unmarshal(e.Data, &snap)
	if snap.LinePrize != line || snap.BingoPrize != bingo {
		t.Fatalf("late snapshot prizes = %+v / %+v, want %+v / %+v",
			snap.LinePrize, snap.BingoPrize, line, bingo)
	}
}

func TestSetPrizesRejectedDuringActive(t *testing.T) {
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

	sendAction(t, adminConn, MsgStart)
	// drain start snapshots (and admin's host_progress meter refresh)
	readEnv(t, adminConn)
	drainHostProgress(t, adminConn)
	readEnv(t, bob)

	line := store.Prize{Enabled: true, Name: "Vino"}
	bingo := store.Prize{Enabled: true, Name: "$5000"}
	sendActionWithData(t, adminConn, MsgSetPrizes, SetPrizesData{Line: line, Bingo: bingo})

	if e := readEnv(t, adminConn); e.Type != MsgError {
		t.Fatalf("set_prizes during active should error, got %q", e.Type)
	}
}

func TestHostProgressOnStartAndDraw(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	bob := joinWS(t, wsURL, room.ID, "bob", "")
	defer bob.Close()
	drainSnapshotAndList(t, bob)
	if e := readEnv(t, adminConn); e.Type != MsgPlayerList {
		t.Fatalf("admin expected player_list, got %q", e.Type)
	}

	sendAction(t, adminConn, MsgStart)
	if e := readEnv(t, adminConn); e.Type != MsgSnapshot {
		t.Fatalf("admin expected snapshot, got %q", e.Type)
	}
	if e := readEnv(t, bob); e.Type != MsgSnapshot {
		t.Fatalf("bob expected snapshot, got %q", e.Type)
	}

	// Admin gets host_progress right after the snapshot.
	e := readEnv(t, adminConn)
	if e.Type != MsgHostProgress {
		t.Fatalf("admin after start expected host_progress, got %q", e.Type)
	}
	var hp HostProgressData
	json.Unmarshal(e.Data, &hp)
	if len(hp.Players) != 1 {
		t.Fatalf("host_progress players = %d, want 1 (bob)", len(hp.Players))
	}
	if hp.Players[0].ToLine != 5 || hp.Players[0].ToBingo != 15 {
		t.Fatalf("fresh card progress = (line %d, bingo %d), want (5, 15)",
			hp.Players[0].ToLine, hp.Players[0].ToBingo)
	}

	// Draw a number — admin gets number_drawn then a refreshed host_progress.
	sendAction(t, adminConn, MsgDraw)
	if e := readEnv(t, adminConn); e.Type != MsgNumberDrawn {
		t.Fatalf("admin after draw expected number_drawn, got %q", e.Type)
	}
	e = readEnv(t, adminConn)
	if e.Type != MsgHostProgress {
		t.Fatalf("admin after draw expected host_progress, got %q", e.Type)
	}

	// Bob (non-admin) must NOT receive host_progress; the next read should be
	// number_drawn (broadcast during the draw), then the connection idles.
	if e := readEnv(t, bob); e.Type != MsgNumberDrawn {
		t.Fatalf("bob after draw expected number_drawn, got %q", e.Type)
	}
	bob.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, _, err := bob.ReadMessage(); err == nil {
		t.Fatalf("bob unexpectedly received another message after number_drawn")
	}
}

func TestSetPrizesNonAdminRejected(t *testing.T) {
	st, wsURL, closeFn := newWSServer(t)
	defer closeFn()
	room, admin, _ := st.CreateRoom("admin")

	adminConn := joinWS(t, wsURL, room.ID, "", string(admin.Token))
	defer adminConn.Close()
	drainSnapshotAndList(t, adminConn)

	bob := joinWS(t, wsURL, room.ID, "bob", "")
	defer bob.Close()
	drainSnapshotAndList(t, bob)
	readEnv(t, adminConn)

	line := store.Prize{Enabled: true, Name: "Vino"}
	bingo := store.Prize{Enabled: true, Name: "$5000"}
	sendActionWithData(t, bob, MsgSetPrizes, SetPrizesData{Line: line, Bingo: bingo})

	if e := readEnv(t, bob); e.Type != MsgError {
		t.Fatalf("non-admin set_prizes should error, got %q", e.Type)
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