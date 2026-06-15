package ws

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/facu/bingo-back/store"
)

func TestPlayerToDTO(t *testing.T) {
	admin := store.Player{ID: "a1", Name: "Ana", Token: "secret-a", Connected: true}
	other := store.Player{ID: "b2", Name: "Beto", Token: "secret-b", Connected: false}

	ai := PlayerToDTO(admin, "a1")
	if ai.ID != "a1" || ai.Name != "Ana" {
		t.Fatalf("bad fields: %+v", ai)
	}
	if !ai.IsAdmin {
		t.Error("admin should have IsAdmin = true")
	}
	if !ai.IsConnected {
		t.Error("admin should have IsConnected = true")
	}

	bi := PlayerToDTO(other, "a1")
	if bi.IsAdmin {
		t.Error("non-admin should have IsAdmin = false")
	}
	if bi.IsConnected {
		t.Error("disconnected player should have IsConnected = false")
	}
}

func TestPlayerInfoJSONHasNoToken(t *testing.T) {
	p := store.Player{ID: "x", Name: "X", Token: "topsecret", Connected: true}

	b, err := json.Marshal(PlayerToDTO(p, "x"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)

	if strings.Contains(s, "topsecret") || strings.Contains(strings.ToLower(s), "token") {
		t.Fatalf("DTO JSON leaks token: %s", s)
	}
	for _, key := range []string{`"id"`, `"name"`, `"isAdmin"`, `"isConnected"`} {
		if !strings.Contains(s, key) {
			t.Errorf("missing expected key %s in %s", key, s)
		}
	}
}

func TestPlayersToDTO(t *testing.T) {
	players := []store.Player{
		{ID: "a1", Name: "Ana", Connected: true},
		{ID: "b2", Name: "Beto", Connected: false},
	}
	got := PlayersToDTO(players, "a1")
	if len(got) != 2 {
		t.Fatalf("got %d infos, want 2", len(got))
	}
}