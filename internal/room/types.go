package room

import (
	"errors"
	"time"

	"github.com/Frankhuex/sanguogao/model"
)

const (
	DefaultMaxPlayers = 10
	RoomCodeLength    = 6
	MaxNameRunes      = 16
)

var ErrCodeExhausted = errors.New("failed to generate unique room code")

// Player 表示在线玩家及其当前所在房间
type Player struct {
	ID       string
	Name     string
	RoomCode string
}

// Room 表示一个房间
type Room struct {
	Code       string
	HostID     string
	Players    []*Player
	MaxPlayers int
	CreatedAt  time.Time
}

func snapshotRoom(r *Room) *model.RoomInfo {
	players := make([]*model.PlayerInfo, len(r.Players))
	for i, p := range r.Players {
		players[i] = &model.PlayerInfo{
			Id:   p.ID,
			Name: p.Name,
		}
	}
	return &model.RoomInfo{
		Code:       r.Code,
		HostId:     r.HostID,
		Players:    players,
		MaxPlayers: int32(r.MaxPlayers),
	}
}

func allIDs(r *Room) []string {
	ids := make([]string, len(r.Players))
	for i, p := range r.Players {
		ids[i] = p.ID
	}
	return ids
}

func otherIDs(r *Room, excludeID string) []string {
	ids := make([]string, 0, len(r.Players))
	for _, p := range r.Players {
		if p.ID != excludeID {
			ids = append(ids, p.ID)
		}
	}
	return ids
}
