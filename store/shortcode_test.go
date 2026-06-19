package store

import (
	"strings"
	"testing"
	"time"
)

func TestCreateRoom_AssignsShortCode(t *testing.T) {
	s := NewStore()
	room, _, err := s.CreateRoom("admin")
	if err != nil {
		t.Fatalf("CreateRoom error: %v", err)
	}
	if len(room.ShortCode) != shortCodeLen {
		t.Fatalf("ShortCode = %q, want %d chars", room.ShortCode, shortCodeLen)
	}
	for _, c := range room.ShortCode {
		if !strings.ContainsRune(shortCodeAlphabet, c) {
			t.Fatalf("ShortCode %q contains rune %q outside the alphabet", room.ShortCode, c)
		}
	}
}

func TestRoomByShortCode_ResolvesAndRespectsMiss(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")

	got, err := s.RoomByShortCode(room.ShortCode)
	if err != nil {
		t.Fatalf("RoomByShortCode error: %v", err)
	}
	if got.ID != room.ID {
		t.Fatalf("RoomByShortCode returned %s, want %s", got.ID, room.ID)
	}

	if _, err := s.RoomByShortCode("ZZZZZ"); err == nil {
		t.Fatal("RoomByShortCode on miss: expected error")
	}
}

func TestShortCodes_AreUniqueAcrossManyRooms(t *testing.T) {
	s := NewStore()
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		room, _, err := s.CreateRoom("admin")
		if err != nil {
			t.Fatalf("CreateRoom #%d: %v", i, err)
		}
		if seen[room.ShortCode] {
			t.Fatalf("duplicate ShortCode %q after %d rooms", room.ShortCode, i)
		}
		seen[room.ShortCode] = true
	}
}

func TestRemoveRoom_ClearsShortCodeAlias(t *testing.T) {
	s := NewStore()
	room, _, _ := s.CreateRoom("admin")
	code := room.ShortCode

	s.RemoveRoom(room.ID)

	if _, err := s.RoomByShortCode(code); err == nil {
		t.Fatal("RoomByShortCode after RemoveRoom: expected error (alias should be cleared)")
	}
}

func TestCleanupInactiveRooms_ClearsShortCodeAlias(t *testing.T) {
	s := NewStore()
	old, _, _ := s.CreateRoom("a")
	code := old.ShortCode
	old.lastAct = time.Now().Add(-time.Hour)

	s.CleanupInactiveRooms(30 * time.Minute)

	if _, err := s.RoomByShortCode(code); err == nil {
		t.Fatal("RoomByShortCode after Cleanup: expected error (alias should be cleared)")
	}
}
