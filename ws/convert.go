package ws

import (
	"github.com/facu/bingo-back/dto"
	"github.com/facu/bingo-back/store"
)

// PlayerToDTO converts a domain Player into the public DTO, dropping the token
// and computing isAdmin against the room's admin id.
func PlayerToDTO(p store.Player, adminID store.PlayerID) dto.PlayerInfo {
	return dto.PlayerInfo{
		ID:          string(p.ID),
		Name:        p.Name,
		IsAdmin:     p.ID == adminID,
		IsConnected: p.Connected,
	}
}

// PlayersToDTO converts a slice of players (e.g. from Room.SnapshotPlayers).
func PlayersToDTO(players []store.Player, adminID store.PlayerID) []dto.PlayerInfo {
	out := make([]dto.PlayerInfo, 0, len(players))
	for _, p := range players {
		out = append(out, PlayerToDTO(p, adminID))
	}
	return out
}