package store

import (
	"testing"

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
		if p.Card == (card.Card{}) {
			t.Fatalf("player %s has no card after Start", id)
		}
	}
}

func TestDrawNextLineAndBingoTie(t *testing.T) {
	s := NewStore()
	room, admin, _ := s.CreateRoom("admin")
	b, _ := s.AddPlayer(room.ID, "b")
	if err := room.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}

	// Make the outcome deterministic: identical card for both players and a
	// controlled bag order (top row first, so the line completes on draw 5 and
	// the full card on draw 15).
	for _, p := range room.players {
		p.Card = testCard
	}
	room.bag = []int{10, 32, 42, 61, 80, 24, 35, 58, 71, 89, 2, 18, 47, 59, 78}
	room.drawn = nil
	room.lineAwarded = false
	room.state = StateActive

	want := map[PlayerID]bool{admin.ID: true, b.ID: true}

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
		if p.Card == (card.Card{}) {
			t.Fatalf("player %s has no card after Restart", id)
		}
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