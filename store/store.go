package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/facu/bingo-back/card"
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

const maxPlayers = 50

// Player holds the information required for game logic and reconnection
type Player struct {
	ID        PlayerID
	Name      string
	Token     Token
	Card      card.Card
	Connected bool
}

// Room represents a multiplayer bingo session
type Room struct {
	ID      RoomID
	AdminID PlayerID
	players map[PlayerID]*Player
	state   JoinState

	// lastAct updates on every visible activity (join/leave, admin action, number draw)
	lastAct time.Time
	mu      sync.Mutex // protects players, state, lastAct
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
		players: make(map[PlayerID]*Player),
		state:   StateIdle,
		lastAct: time.Now(),
	}
}

func (r *Room) SnapshotPlayers() []Player {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Player, 0, len(r.players))
	for _, p := range r.players {
		out = append(out, *p)
	}
	return out
}

func (r *Room) State() JoinState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state
}

func (r *Room) SetState(s JoinState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state = s
	r.lastAct = time.Now()
}

func (r *Room) MarkDisconnected(pid PlayerID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.players[pid]; ok {
		p.Connected = false
	}
	r.lastAct = time.Now()
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

func (s *Store) CreateRoom(adminName string) (*Room, *Player, error) {
	pid := PlayerID(createUUID())
	token, err := genToken()
	if err != nil {
		return nil, nil, err
	}
	admin := &Player{ID: pid, Name: adminName, Token: token}
	room := NewRoom(pid)
	room.players[pid] = admin
	if err := s.AddRoom(room); err != nil {
		return nil, nil, err
	}
	return room, admin, nil
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
	player := &Player{ID: pid, Name: name, Token: token, Connected: true}
	room.mu.Lock()
	defer room.mu.Unlock()
	if len(room.players) >= maxPlayers {
		return nil, fmt.Errorf("room %s is full", roomID)
	}
	room.players[pid] = player
	room.lastAct = time.Now()
	return player, nil
}

func (s *Store) Reconnect(roomID RoomID, t Token) (*Player, error) {
	room, err := s.GetRoom(roomID)
	if err != nil {
		return nil, err
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	for _, p := range room.players {
		if p.Token == t {
			p.Connected = true
			room.lastAct = time.Now()
			return p, nil
		}
	}
	return nil, fmt.Errorf("player token not found in room %s", roomID)
}

// RemovePlayer drops a player from the room. No error if player absent.
func (s *Store) RemovePlayer(roomID RoomID, pid PlayerID) {
	room, err := s.GetRoom(roomID)
	if err != nil {
		return
	}
	room.mu.Lock()
	defer room.mu.Unlock()
	delete(room.players, pid)
	room.lastAct = time.Now()
}

// CleanupInactiveRooms removes rooms that have been idle longer than d and are not in an active state.
func (s *Store) CleanupInactiveRooms(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for id, r := range s.rooms {
		r.mu.Lock()
		idle := now.Sub(r.lastAct) > d && r.state == StateIdle
		r.mu.Unlock()
		if idle {
			delete(s.rooms, id)
		}
	}
}