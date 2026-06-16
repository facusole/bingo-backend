package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/facu/bingo-back/card"
	"github.com/facu/bingo-back/game"

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
	lastAct     time.Time
	bag         []int
	drawn       []int
	lineAwarded bool
	mu          sync.Mutex // protects players, state, lastAct, bag, drawn, lineAwarded
}

// DrawResult is the outcome of a single number draw.
type DrawResult struct {
	Number       int
	LineWinners  []PlayerID // non-empty only on the draw that awards the line
	BingoWinners []PlayerID // non-empty when one or more cards are complete
	Finished     bool       // true when this draw produced a bingo winner
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

// resetGame reassigns cards, refills the bag and sets the room active.
// The caller must hold r.mu.
func (r *Room) resetGame() error {
	for _, p := range r.players {
		c, err := card.GenerateCard()
		if err != nil {
			return err
		}
		p.Card = c
	}
	r.bag = game.NewBag()
	r.drawn = nil
	r.lineAwarded = false
	r.state = StateActive
	r.lastAct = time.Now()
	return nil
}

// Start begins a new game: fresh cards for every player, a new bag, state active.
func (r *Room) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resetGame()
}

// Restart begins a new game in the same room with the players still present.
func (r *Room) Restart() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.resetGame()
}

// DrawNext pops the next number, records it and evaluates prizes atomically.
func (r *Room) DrawNext() (DrawResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.state != StateActive {
		return DrawResult{}, fmt.Errorf("room %s is not active", r.ID)
	}
	if len(r.bag) == 0 {
		return DrawResult{}, fmt.Errorf("room %s has no numbers left", r.ID)
	}

	n := r.bag[0]
	r.bag = r.bag[1:]
	r.drawn = append(r.drawn, n)
	r.lastAct = time.Now()

	res := DrawResult{Number: n}
	for _, p := range r.players {
		// skip players with no card yet (shouldn't happen while active)
		if p.Card == (card.Card{}) {
			continue
		}
		if !r.lineAwarded && game.HasLine(p.Card, r.drawn) {
			res.LineWinners = append(res.LineWinners, p.ID)
		}
		if game.CardComplete(p.Card, r.drawn) {
			res.BingoWinners = append(res.BingoWinners, p.ID)
		}
	}
	if len(res.LineWinners) > 0 {
		r.lineAwarded = true
	}
	if len(res.BingoWinners) > 0 {
		res.Finished = true
		r.state = StateFinished
	}

	return res, nil
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
	if room.state == StateActive {
		// generate a card now; regenerate in the rare case it is already complete
		for {
			c, err := card.GenerateCard()
			if err != nil {
				return nil, err
			}
			if !game.CardComplete(c, room.drawn) {
				player.Card = c
				break
			}
		}
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

// CleanupInactiveRooms removes rooms with no connected players that have been idle longer than d.
func (s *Store) CleanupInactiveRooms(d time.Duration) []RoomID {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var removed []RoomID
	for id, r := range s.rooms {
		r.mu.Lock()
		connected := 0
		for _, p := range r.players {
			if p.Connected {
				connected++
			}
		}
		idle := now.Sub(r.lastAct) > d && connected == 0
		r.mu.Unlock()
		if idle {
			delete(s.rooms, id)
			removed = append(removed, id)
		}
	}
	return removed
}