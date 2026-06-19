package store

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
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

// Prize describes a named, optional reward for line or bingo. Informative only —
// the server tracks the configured prize but does not deliver it.
type Prize struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`
}

// Room represents a multiplayer bingo session
type Room struct {
	ID        RoomID
	ShortCode string
	AdminID   PlayerID
	players   map[PlayerID]*Player
	state     JoinState

	// lastAct updates on every visible activity (join/leave, admin action, number draw)
	lastAct     time.Time
	bag         []int
	drawn       []int
	lineAwarded bool
	linePrize   Prize
	bingoPrize  Prize
	mu          sync.Mutex // protects players, state, lastAct, bag, drawn, lineAwarded, linePrize, bingoPrize
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
	rooms     map[RoomID]*Room
	shortCode map[string]RoomID // shareable 5-char alias -> roomID
	mu        sync.RWMutex      // protects rooms and shortCode
}

// NewStore creates an empty store
func NewStore() *Store {
	return &Store{
		rooms:     make(map[RoomID]*Room),
		shortCode: make(map[string]RoomID),
	}
}

// shortCodeAlphabet is the Crockford-style alphabet used for room codes:
// no 0/O or 1/I to avoid visual ambiguity when sharing.
const shortCodeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
const shortCodeLen = 5

// generateShortCode returns a random 5-char code drawn from shortCodeAlphabet.
func generateShortCode() (string, error) {
	b := make([]byte, shortCodeLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := make([]byte, shortCodeLen)
	for i, v := range b {
		out[i] = shortCodeAlphabet[int(v)%len(shortCodeAlphabet)]
	}
	return string(out), nil
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
// The admin is the host and does not play, so they receive no card.
// The caller must hold r.mu.
func (r *Room) resetGame() error {
	for _, p := range r.players {
		if p.ID == r.AdminID {
			p.Card = card.Card{}
			continue
		}
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

// PlayerProgress reports how close a player is to line and to bingo. ToLine is
// the minimum number of unmarked cells across the three rows; ToBingo is the
// total unmarked cells (out of the 15 numbers on the card).
type PlayerProgress struct {
	PlayerID PlayerID `json:"playerId"`
	ToLine   int      `json:"toLine"`
	ToBingo  int      `json:"toBingo"`
}

// PlayersProgress returns the distance-to-win for every non-admin player.
// Sorted by PlayerID for a STABLE backend contract — UX ordering ("hottest
// player first") is the frontend's job in the tension meter; the backend
// stays presentation-agnostic so tests and replays are deterministic.
func (r *Room) PlayersProgress() []PlayerProgress {
	r.mu.Lock()
	defer r.mu.Unlock()
	drawn := make(map[int]bool, len(r.drawn))
	for _, n := range r.drawn {
		drawn[n] = true
	}
	out := make([]PlayerProgress, 0, len(r.players))
	for _, p := range r.players {
		if p.ID == r.AdminID {
			continue
		}
		if p.Card == (card.Card{}) {
			continue
		}
		marked := 0
		minMissing := 6 // any real row has 0-5 missing, so first row always wins
		for row := 0; row < 3; row++ {
			missing := 0
			for col := 0; col < 9; col++ {
				n := p.Card[row][col]
				if n == 0 {
					continue
				}
				if drawn[n] {
					marked++
				} else {
					missing++
				}
			}
			if missing < minMissing {
				minMissing = missing
			}
		}
		out = append(out, PlayerProgress{
			PlayerID: p.ID,
			ToLine:   minMissing,
			ToBingo:  15 - marked,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].PlayerID < out[j].PlayerID
	})
	return out
}

// SetPrizes updates the room's line and bingo prizes. Only allowed in idle
// or finished state — changing prizes mid-game would surprise players who
// already committed to playing for the announced reward.
func (r *Room) SetPrizes(line, bingo Prize) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.state == StateActive {
		return fmt.Errorf("cannot change prizes while game is active")
	}
	r.linePrize = line
	r.bingoPrize = bingo
	r.lastAct = time.Now()
	return nil
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
		// admin is host, never competes; check is mandatory because an empty
		// card would satisfy HasLine/CardComplete vacuously.
		if p.ID == r.AdminID {
			continue
		}
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
	if r.ShortCode != "" {
		s.shortCode[r.ShortCode] = r.ID
	}
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

// RoomByShortCode resolves the shareable 5-char alias to the underlying room.
// Returns an error when the code is not registered.
func (s *Store) RoomByShortCode(code string) (*Room, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rid, ok := s.shortCode[code]
	if !ok {
		return nil, fmt.Errorf("short code %s not found", code)
	}
	r, ok := s.rooms[rid]
	if !ok {
		return nil, fmt.Errorf("room %s not found", rid)
	}
	return r, nil
}

// RemoveRoom deletes a room from the store, including its short-code alias.
func (s *Store) RemoveRoom(id RoomID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.rooms[id]; ok && r.ShortCode != "" {
		delete(s.shortCode, r.ShortCode)
	}
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

	// Generate a unique short code; retry on collision.
	for attempt := 0; attempt < 32; attempt++ {
		code, err := generateShortCode()
		if err != nil {
			return nil, nil, err
		}
		s.mu.RLock()
		_, taken := s.shortCode[code]
		s.mu.RUnlock()
		if !taken {
			room.ShortCode = code
			break
		}
	}
	if room.ShortCode == "" {
		return nil, nil, fmt.Errorf("failed to generate a unique short code")
	}

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
		code := r.ShortCode
		r.mu.Unlock()
		if idle {
			delete(s.rooms, id)
			if code != "" {
				delete(s.shortCode, code)
			}
			removed = append(removed, id)
		}
	}
	return removed
}