package store

import (
    "crypto/rand"
    "encoding/base64"
    "fmt"
    "sync"
    "time"

    "github.com/google/uuid"
)

// Type aliases for clearer domain intent
type RoomID string
type PlayerID string
type Token string

// JoinState represents the lifecycle of a game in a room
type JoinState string

const (
    StateIdle     JoinState = "idle"
    StateActive   JoinState = "active"
    StateFinished JoinState = "finished"
)

// Card represents a 3x9 bingo card. A nil pointer means an empty cell.
type Card [3][9]*int

// Player holds the information required for game logic and reconnection
type Player struct {
    ID    PlayerID
    Name  string
    Token Token
    Card  Card
}

// Room represents a multiplayer bingo session
type Room struct {
    ID      RoomID
    AdminID PlayerID
    Players map[PlayerID]*Player
    State   JoinState

    // lastAct updates on every visible activity (join/leave, admin action, number draw)
    lastAct time.Time
    mu      sync.Mutex // protects Players, State, lastAct
}

// Store keeps all active rooms in memory
type Store struct {
    rooms map[RoomID]*Room
    mu    sync.RWMutex // protects the rooms map
}

// NewStore creates an empty store
func NewStore() *Store {
    return &Store{rooms: make(map[RoomID]*Room)}
}

// createUUID returns a new UUID string
func createUUID() string {
    return uuid.NewString()
}

// genToken generates a URL safe reconnection token
func genToken() (Token, error) {
    const size = 32
    b := make([]byte, size)
    if _, err := rand.Read(b); err != nil {
        return "", err
    }
    return Token(base64.RawURLEncoding.EncodeToString(b)), nil
}

// NewRoom constructs a room with a unique UUID and sets its initial state
func NewRoom(adminID PlayerID) *Room {
    rid := RoomID(createUUID())
    return &Room{
        ID:      rid,
        AdminID: adminID,
        Players: make(map[PlayerID]*Player),
        State:   StateIdle,
        lastAct: time.Now(),
    }
}

// AddRoom inserts a new room into the store. Returns an error if the ID already exists.
func (s *Store) AddRoom(r *Room) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    if _, exists := s.rooms[r.ID]; exists {
        return fmt.Errorf("room %s already exists", r.ID)
    }
    s.rooms[r.ID] = r
    return nil
}

// GetRoom retrieves a room by ID. Returns an error if not found.
func (s *Store) GetRoom(id RoomID) (*Room, error) {
    s.mu.RLock()
    defer s.mu.RUnlock()
    r, ok := s.rooms[id]
    if !ok {
        return nil, fmt.Errorf("room %s not found", id)
    }
    return r, nil
}

// RemoveRoom deletes a room from the store
func (s *Store) RemoveRoom(id RoomID) {
    s.mu.Lock()
    defer s.mu.Unlock()
    delete(s.rooms, id)
}

// AddPlayer creates a player for the given room and returns the player object
func (s *Store) AddPlayer(roomID RoomID, name string) (*Player, error) {
    room, err := s.GetRoom(roomID)
    if err != nil {
        return nil, err
    }
    pid := PlayerID(createUUID())
    token, err := genToken()
    if err != nil {
        return nil, err
    }
    player := &Player{ID: pid, Name: name, Token: token}
    room.mu.Lock()
    defer room.mu.Unlock()
    room.Players[pid] = player
    room.lastAct = time.Now()
    return player, nil
}

// RemovePlayer drops a player from the room. No error if player absent.
func (s *Store) RemovePlayer(roomID RoomID, pid PlayerID) {
    room, err := s.GetRoom(roomID)
    if err != nil {
        return
    }
    room.mu.Lock()
    defer room.mu.Unlock()
    delete(room.Players, pid)
    room.lastAct = time.Now()
}

// CleanupInactiveRooms removes rooms that have been idle longer than d and are not in an active state.
func (s *Store) CleanupInactiveRooms(d time.Duration) {
    s.mu.Lock()
    defer s.mu.Unlock()
    now := time.Now()
    for id, r := range s.rooms {
        r.mu.Lock()
        idle := now.Sub(r.lastAct) > d && r.State == StateIdle
        r.mu.Unlock()
        if idle {
            delete(s.rooms, id)
        }
    }
}
