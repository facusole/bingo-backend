package ws

import (
	"encoding/json"

	"github.com/facu/bingo-back/card"
	"github.com/facu/bingo-back/store"
)

// session holds the per-connection context used to dispatch incoming messages.
type session struct {
	handler *Handler
	room    *store.Room
	hub     *Hub
	client  *Client
}

// handle routes an incoming message to its action. Unknown types are ignored.
func (s *session) handle(msg Incoming) {
	switch msg.Type {
	case MsgStart:
		s.start()
	case MsgDraw:
		s.draw()
	case MsgRestart:
		s.restart()
	case MsgClose:
		s.closeRoom()
	case MsgSetPrizes:
		s.setPrizes(msg.Data)
	}
}

// requireAdmin returns true only if this connection belongs to the room admin;
// otherwise it sends an error to the caller and returns false.
func (s *session) requireAdmin() bool {
	if s.client.PlayerID != s.room.AdminID {
		s.sendError("only the admin can do that")
		return false
	}
	return true
}

func (s *session) start() {
	if !s.requireAdmin() {
		return
	}
	if err := s.room.Start(); err != nil {
		s.sendError(err.Error())
		return
	}
	s.dealSnapshots()
	sendHostProgress(s.hub, s.client, s.room)
}

func (s *session) restart() {
	if !s.requireAdmin() {
		return
	}
	if err := s.room.Restart(); err != nil {
		s.sendError(err.Error())
		return
	}
	s.dealSnapshots()
	sendHostProgress(s.hub, s.client, s.room)
}

func (s *session) draw() {
	if !s.requireAdmin() {
		return
	}
	res, err := s.room.DrawNext()
	if err != nil {
		s.sendError(err.Error())
		return
	}
	if msg, err := Encode(MsgNumberDrawn, NumberDrawnData{Number: res.Number}); err == nil {
		s.hub.Broadcast(msg)
	}
	if len(res.LineWinners) > 0 {
		if msg, err := Encode(MsgLineAwarded, WinnersData{Winners: idStrings(res.LineWinners)}); err == nil {
			s.hub.Broadcast(msg)
		}
	}
	if len(res.BingoWinners) > 0 {
		if msg, err := Encode(MsgBingoAwarded, WinnersData{Winners: idStrings(res.BingoWinners)}); err == nil {
			s.hub.Broadcast(msg)
		}
	}
	// host-only tension meter: refresh after every draw, since the admin is
	// the one who fires draw() they are this connection (s.client).
	sendHostProgress(s.hub, s.client, s.room)
}

func (s *session) setPrizes(data json.RawMessage) {
	if !s.requireAdmin() {
		return
	}
	var sp SetPrizesData
	if len(data) == 0 {
		s.sendError("set_prizes requires a payload")
		return
	}
	if err := json.Unmarshal(data, &sp); err != nil {
		s.sendError("invalid set_prizes payload")
		return
	}
	if err := s.room.SetPrizes(sp.Line, sp.Bingo); err != nil {
		s.sendError(err.Error())
		return
	}
	if msg, err := Encode(MsgPrizesUpdated, PrizesUpdatedData{Line: sp.Line, Bingo: sp.Bingo}); err == nil {
		s.hub.Broadcast(msg)
	}
}

func (s *session) closeRoom() {
	if !s.requireAdmin() {
		return
	}
	// Broadcast first so the message lands in each mailbox before the hub stops;
	// the write pump drains it before sending the close frame.
	if msg, err := Encode(MsgRoomClosed, nil); err == nil {
		s.hub.Broadcast(msg)
	}
	s.handler.Hubs.Remove(s.room.ID)
	s.handler.Store.RemoveRoom(s.room.ID)
}

// dealSnapshots sends every player a fresh snapshot with their own new card.
// Reusing the snapshot means the client needs no new logic to start a game.
func (s *session) dealSnapshots() {
	players := s.room.SnapshotPlayers()
	dtos := PlayersToDTO(players, s.room.AdminID)
	drawn := s.room.Drawn()
	lineAwarded := s.room.LineAwarded()
	state := string(s.room.State())
	linePrize, bingoPrize := s.room.Prizes()

	byID := make(map[store.PlayerID][]byte, len(players))
	for _, p := range players {
		var cardPtr *card.Card
		if p.ID != s.room.AdminID {
			pc := p.Card
			cardPtr = &pc
		}
		snap := SnapshotData{
			PlayerID:    string(p.ID),
			Token:       string(p.Token),
			IsAdmin:     p.ID == s.room.AdminID,
			Card:        cardPtr,
			Drawn:       drawn,
			LineAwarded: lineAwarded,
			State:       state,
			Players:     dtos,
			LinePrize:   linePrize,
			BingoPrize:  bingoPrize,
		}
		if msg, err := Encode(MsgSnapshot, snap); err == nil {
			byID[p.ID] = msg
		}
	}
	s.hub.SendEach(byID)
}

func (s *session) sendError(message string) {
	if msg, err := Encode(MsgError, ErrorData{Message: message}); err == nil {
		s.hub.SendTo(s.client, msg)
	}
}

func idStrings(ids []store.PlayerID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}