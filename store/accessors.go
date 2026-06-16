package store

import "github.com/facu/bingo-back/card"

// Drawn returns a copy of the numbers drawn so far, in order.
func (r *Room) Drawn() []int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]int, len(r.drawn))
	copy(out, r.drawn)
	return out
}

// LineAwarded reports whether the line prize has already been granted.
func (r *Room) LineAwarded() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lineAwarded
}

// PlayerCard returns a copy of a player's card, or the zero card if absent.
func (r *Room) PlayerCard(pid PlayerID) card.Card {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.players[pid]; ok {
		return p.Card
	}
	return card.Card{}
}