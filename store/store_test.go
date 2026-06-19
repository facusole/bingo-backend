package store

import (
	"testing"
	"time"

	"github.com/facu/bingo-back/card"
)

// a fixed 90-ball card used to make game outcomes deterministic
var testCard = card.Card{
	{0, 10, 0, 32, 42, 0, 61, 0, 80},
	{0, 0, 24, 35, 0, 58, 0, 71, 89},
	{2, 18, 0, 0, 47, 59, 0, 78, 0},
}

func TestCreateRoom(t *testing.T) {
	s := NewStore()
	room, admin, err := s.CreateRoom("facu")
	if err != nil {
		t.Fatalf("CreateRoom error: %v", err)
	}
	if room.AdminID != admin.ID {
		t.Fatalf("AdminID %q != admin.ID %q", room.AdminID, admin.ID)
	}
	if admin.Token == "" {
		t.Fatal("admin token is empty")
	}
	if room.State() != StateIdle {
		t.Fatalf("new room state = %q, want idle", room.State())
	}
	if _, ok := room.players[admin.ID]; !ok {
		t.Fatal("admin not present in room players")
	}
}

func TestAddPlayerCap(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin") // room starts with 1 player (the admin)
	added := 1
	for {
		if _, err := s.AddPlayer(room.ID, "p"); err != nil {
			break
		}
		added++
		if added > maxPlayers+5 { // guard against an infinite loop if the cap breaks
			t.Fatal("AddPlayer never returned a 'full' error")
		}
	}
	if added != maxPlayers {
		t.Fatalf("room filled with %d players, want %d", added, maxPlayers)
	}
}

func TestStartAssignsCards(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	s.AddPlayer(room.ID, "b")

	if err := room.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	if room.State() != StateActive {
		t.Fatalf("state after Start = %q, want active", room.State())
	}
	if len(room.bag) != 90 {
		t.Fatalf("bag len after Start = %d, want 90", len(room.bag))
	}
	if len(room.drawn) != 0 {
		t.Fatalf("drawn after Start = %d, want 0", len(room.drawn))
	}
	if room.lineAwarded {
		t.Fatal("lineAwarded should be false after Start")
	}
	for id, p := range room.players {
		if id == room.AdminID {
			if p.Card != (card.Card{}) {
				t.Fatalf("admin %s should have no card after Start", id)
			}
			continue
		}
		if p.Card == (card.Card{}) {
			t.Fatalf("player %s has no card after Start", id)
		}
	}
}

func TestDrawNextLineAndBingoTie(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	b, _ := s.AddPlayer(room.ID, "b")
	c, _ := s.AddPlayer(room.ID, "c")
	if err := room.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Make the outcome deterministic: identical card for both non-admin players
	// and a controlled bag order (top row first, so the line completes on draw 5
	// and the full card on draw 15). Admin keeps an empty card (host, not a
	// player) and must never appear in winners.
	for id, p := range room.players {
		if id == room.AdminID {
			continue
		}
		p.Card = testCard
	}
	room.bag = []int{10, 32, 42, 61, 80, 24, 35, 58, 71, 89, 2, 18, 47, 59, 78}
	room.drawn = nil
	room.lineAwarded = false
	room.state = StateActive

	want := map[PlayerID]bool{b.ID: true, c.ID: true}

	// draws 1-4: nothing awarded yet
	for i := 0; i < 4; i++ {
		res := mustDraw(t, room)
		if len(res.LineWinners) != 0 || len(res.BingoWinners) != 0 || res.Finished {
			t.Fatalf("draw %d: unexpected result %+v", i+1, res)
		}
	}

	// draw 5 completes the top row -> line for both, exactly once
	res := mustDraw(t, room)
	assertSameSet(t, "line winners", res.LineWinners, want)
	if len(res.BingoWinners) != 0 || res.Finished {
		t.Fatalf("draw 5: bingo awarded too early: %+v", res)
	}

	// draws 6-14: line already awarded, bingo not yet
	for i := 5; i < 14; i++ {
		res := mustDraw(t, room)
		if len(res.LineWinners) != 0 {
			t.Fatalf("draw %d: line awarded a second time", i+1)
		}
		if len(res.BingoWinners) != 0 || res.Finished {
			t.Fatalf("draw %d: bingo awarded too early: %+v", i+1, res)
		}
	}

	// draw 15 completes the card -> bingo tie for both, game finished
	res = mustDraw(t, room)
	assertSameSet(t, "bingo winners", res.BingoWinners, want)
	if !res.Finished {
		t.Fatal("draw 15: Finished = false, want true")
	}
	if len(res.LineWinners) != 0 {
		t.Fatal("draw 15: line should not be re-awarded")
	}
	if room.State() != StateFinished {
		t.Fatalf("state after bingo = %q, want finished", room.State())
	}
}

func TestDrawNextErrors(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")

	// room is still idle
	if _, err := room.DrawNext(); err == nil {
		t.Fatal("DrawNext on idle room: expected error")
	}

	// active but with an empty bag
	room.SetState(StateActive)
	room.bag = nil
	if _, err := room.DrawNext(); err == nil {
		t.Fatal("DrawNext with empty bag: expected error")
	}
}

func TestRestartResets(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	s.AddPlayer(room.ID, "b")
	room.Start()

	// simulate a finished game with some state to clear
	room.SetState(StateFinished)
	room.drawn = []int{1, 2, 3}
	room.lineAwarded = true

	if err := room.Restart(); err != nil {
		t.Fatalf("Restart error: %v", err)
	}
	if room.State() != StateActive {
		t.Fatalf("state after Restart = %q, want active", room.State())
	}
	if len(room.bag) != 90 {
		t.Fatalf("bag len after Restart = %d, want 90", len(room.bag))
	}
	if len(room.drawn) != 0 {
		t.Fatalf("drawn after Restart = %d, want 0", len(room.drawn))
	}
	if room.lineAwarded {
		t.Fatal("lineAwarded should be false after Restart")
	}
	for id, p := range room.players {
		if id == room.AdminID {
			if p.Card != (card.Card{}) {
				t.Fatalf("admin %s should have no card after Restart", id)
			}
			continue
		}
		if p.Card == (card.Card{}) {
			t.Fatalf("player %s has no card after Restart", id)
		}
	}
}

func TestPlayersProgress(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	b, _ := s.AddPlayer(room.ID, "b")
	c, _ := s.AddPlayer(room.ID, "c")
	if err := room.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Give both non-admin players a deterministic card. The admin keeps an
	// empty card (host, not playing) and must NOT appear in the output.
	for id, p := range room.players {
		if id == room.AdminID {
			continue
		}
		p.Card = testCard
	}

	// Mark 4 of the top-row numbers as drawn — b is one cell away from line,
	// c is identical so same distance.
	room.drawn = []int{10, 32, 42, 61} // missing 80 to complete top row

	prog := room.PlayersProgress()
	if len(prog) != 2 {
		t.Fatalf("PlayersProgress len = %d, want 2 (admin excluded)", len(prog))
	}
	for _, p := range prog {
		if p.PlayerID == room.AdminID {
			t.Fatalf("admin %s leaked into PlayersProgress", room.AdminID)
		}
		if p.ToLine != 1 {
			t.Fatalf("player %s toLine = %d, want 1", p.PlayerID, p.ToLine)
		}
		if p.ToBingo != 11 { // 15 - 4 marked
			t.Fatalf("player %s toBingo = %d, want 11", p.PlayerID, p.ToBingo)
		}
	}

	// Stable order: sorted by PlayerID ascending.
	if !(prog[0].PlayerID < prog[1].PlayerID) {
		t.Fatalf("PlayersProgress not sorted by PlayerID: %v", prog)
	}

	_, _ = b, c
}

func TestSetPrizes(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")

	line := Prize{Enabled: true, Name: "Vino"}
	bingo := Prize{Enabled: true, Name: "$5000"}

	// idle is allowed
	if err := room.SetPrizes(line, bingo); err != nil {
		t.Fatalf("SetPrizes on idle: %v", err)
	}
	gotLine, gotBingo := room.Prizes()
	if gotLine != line || gotBingo != bingo {
		t.Fatalf("Prizes after idle SetPrizes = %+v / %+v, want %+v / %+v", gotLine, gotBingo, line, bingo)
	}

	// active is rejected
	room.SetState(StateActive)
	disallowed := Prize{Enabled: true, Name: "Otro"}
	if err := room.SetPrizes(disallowed, disallowed); err == nil {
		t.Fatal("SetPrizes during active state should fail")
	}
	gotLine, gotBingo = room.Prizes()
	if gotLine != line || gotBingo != bingo {
		t.Fatalf("Prizes were mutated despite active state: %+v / %+v", gotLine, gotBingo)
	}

	// finished is allowed again
	room.SetState(StateFinished)
	again := Prize{Enabled: false, Name: ""}
	if err := room.SetPrizes(again, again); err != nil {
		t.Fatalf("SetPrizes on finished: %v", err)
	}
}

func TestPrizesPersistAcrossRestart(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	s.AddPlayer(room.ID, "b")

	line := Prize{Enabled: true, Name: "Vino"}
	bingo := Prize{Enabled: true, Name: "$5000"}
	if err := room.SetPrizes(line, bingo); err != nil {
		t.Fatalf("SetPrizes: %v", err)
	}
	if err := room.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	room.SetState(StateFinished)
	if err := room.Restart(); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	gotLine, gotBingo := room.Prizes()
	if gotLine != line || gotBingo != bingo {
		t.Fatalf("Prizes after Restart = %+v / %+v, want %+v / %+v", gotLine, gotBingo, line, bingo)
	}
}

// TestAdminNeverWins drains the entire bag with the admin as the only "player"
// candidate, plus a real player. The admin's card stays empty, so a vacuous
// HasLine/CardComplete must NOT add the admin to winners on any draw.
func TestAdminNeverWins(t *testing.T) {
	s := NewStore()
	room, admin, _ := s.CreateRoom("admin")
	b, _ := s.AddPlayer(room.ID, "b")
	if err := room.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	saw := map[PlayerID]bool{}
	for len(room.bag) > 0 {
		res, err := room.DrawNext()
		if err != nil {
			t.Fatalf("DrawNext error: %v", err)
		}
		for _, id := range res.LineWinners {
			saw[id] = true
		}
		for _, id := range res.BingoWinners {
			saw[id] = true
		}
		if res.Finished {
			break
		}
	}
	if saw[admin.ID] {
		t.Fatalf("admin %s appeared in winners; admin must never win", admin.ID)
	}
	if !saw[b.ID] {
		t.Fatalf("player %s did not win after draining the bag", b.ID)
	}
}

// --- helpers ---

func mustDraw(t *testing.T, r *Room) DrawResult {
	t.Helper()
	res, err := r.DrawNext()
	if err != nil {
		t.Fatalf("DrawNext error: %v", err)
	}
	return res
}

func assertSameSet(t *testing.T, label string, got []PlayerID, want map[PlayerID]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %d (%v), want %d", label, len(got), got, len(want))
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("%s: unexpected id %q in %v", label, id, got)
		}
	}
}

func TestCleanupInactiveRooms(t *testing.T) {
	s := NewStore()

	// abandonada: sin nadie conectado y vieja
	old, _, _ := s.CreateRoom("a")
	old.lastAct = time.Now().Add(-time.Hour)

	// viva: tiene un jugador conectado
	live, _, _ := s.CreateRoom("b")
	s.AddPlayer(live.ID, "p") // AddPlayer deja Connected=true

	removed := s.CleanupInactiveRooms(30 * time.Minute)

	if len(removed) != 1 || removed[0] != old.ID {
		t.Fatalf("removed = %v, want [%s]", removed, old.ID)
	}
	if _, err := s.GetRoom(old.ID); err == nil {
		t.Fatal("la sala abandonada debería haberse borrado")
	}
	if _, err := s.GetRoom(live.ID); err != nil {
		t.Fatal("la sala con un jugador conectado debería sobrevivir")
	}
}