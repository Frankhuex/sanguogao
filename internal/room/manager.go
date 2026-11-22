package room

import (
	"crypto/rand"
	"math/big"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Frankhuex/sanguogao/model"
)

const (
	DefaultMaxPlayers = 10
	RoomCodeLength    = 6
	MaxNameRunes      = 16
	maxCodeRetries    = 10
	// 剔除易混淆字符 0/O/1/I/L
	roomCodeAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
)

type Player struct {
	ID       string
	Name     string
	RoomCode string
}

type Room struct {
	Code       string
	HostID     string
	Players    []*Player
	MaxPlayers int
	Playing    bool
	Offline    map[string]bool
	CreatedAt  time.Time
}

type DisconnectResult struct {
	RoomInfo        *model.RoomInfo
	NotifyIDs       []string
	PlayingRoomCode string // 若掉线时正在对局中，返回对应房间号
	AllOffline      bool   // 对局中所有玩家是否均已掉线
}

type Manager struct {
	mu         sync.RWMutex
	maxPlayers int
	rooms      map[string]*Room
	players    map[string]*Player
}

func NewManager(maxPlayers int) *Manager {
	if maxPlayers <= 0 {
		maxPlayers = DefaultMaxPlayers
	}
	return &Manager{
		maxPlayers: maxPlayers,
		rooms:      make(map[string]*Room),
		players:    make(map[string]*Player),
	}
}

// AddPlayer 注册在线玩家并分配默认昵称
func (m *Manager) AddPlayer(clientID string) *model.PlayerInfo {
	suffix := strings.ToUpper(clientID)
	if len(suffix) > 4 {
		suffix = suffix[len(suffix)-4:]
	}
	p := &Player{
		ID:   clientID,
		Name: "游客_" + suffix,
	}

	m.mu.Lock()
	m.players[clientID] = p
	info := &model.PlayerInfo{Id: p.ID, Name: p.Name}
	m.mu.Unlock()

	return info
}

// SetName 设置或修改玩家昵称；若玩家在等待中的房间内，返回房间快照与需要通知的其他成员 ID
func (m *Manager) SetName(clientID, rawName string) (*model.PlayerInfo, *model.RoomInfo, []string, model.ErrorCode, string) {
	name := strings.TrimSpace(rawName)
	runeCount := utf8.RuneCountInString(name)
	if runeCount < 1 || runeCount > MaxNameRunes {
		return nil, nil, nil, model.ErrorCode_ERROR_CODE_INVALID_PARAM, "昵称长度需在 1~16 个字符之间"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return nil, nil, nil, model.ErrorCode_ERROR_CODE_INTERNAL, "玩家状态不存在"
	}
	p.Name = name
	self := &model.PlayerInfo{Id: p.ID, Name: p.Name}

	if p.RoomCode != "" {
		if r, exists := m.rooms[p.RoomCode]; exists && !r.Playing {
			return self, snapshotRoom(r), otherIDs(r, clientID), model.ErrorCode_ERROR_CODE_OK, ""
		}
	}
	return self, nil, nil, model.ErrorCode_ERROR_CODE_OK, ""
}

// CreateRoom 创建房间，返回新房间快照
func (m *Manager) CreateRoom(clientID string) (*model.RoomInfo, model.ErrorCode, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return nil, model.ErrorCode_ERROR_CODE_INTERNAL, "玩家状态不存在"
	}
	if p.RoomCode != "" {
		return nil, model.ErrorCode_ERROR_CODE_ALREADY_IN_ROOM, "你已经在房间中，请先退出当前房间"
	}

	code, err := m.generateUniqueCodeLocked()
	if err != nil {
		return nil, model.ErrorCode_ERROR_CODE_INTERNAL, "生成房间号失败"
	}

	r := &Room{
		Code:       code,
		HostID:     p.ID,
		Players:    []*Player{p},
		MaxPlayers: m.maxPlayers,
		CreatedAt:  time.Now(),
	}
	m.rooms[code] = r
	p.RoomCode = code

	return snapshotRoom(r), model.ErrorCode_ERROR_CODE_OK, ""
}

// JoinRoom 加入房间（房间号大小写不敏感），返回最新房间快照和需要通知的其他成员 ID
func (m *Manager) JoinRoom(clientID, rawCode string) (*model.RoomInfo, []string, model.ErrorCode, string) {
	code := strings.ToUpper(strings.TrimSpace(rawCode))
	if len(code) != RoomCodeLength {
		return nil, nil, model.ErrorCode_ERROR_CODE_INVALID_PARAM, "房间号格式不正确"
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return nil, nil, model.ErrorCode_ERROR_CODE_INTERNAL, "玩家状态不存在"
	}
	if p.RoomCode != "" {
		return nil, nil, model.ErrorCode_ERROR_CODE_ALREADY_IN_ROOM, "你已经在房间中，请先退出当前房间"
	}

	r, exists := m.rooms[code]
	if !exists {
		return nil, nil, model.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND, "房间不存在"
	}
	if r.Playing {
		return nil, nil, model.ErrorCode_ERROR_CODE_ROOM_PLAYING, "房间正在游戏中，无法加入"
	}
	if r.MaxPlayers > 0 && len(r.Players) >= r.MaxPlayers {
		return nil, nil, model.ErrorCode_ERROR_CODE_ROOM_FULL, "房间已满"
	}

	r.Players = append(r.Players, p)
	p.RoomCode = code

	return snapshotRoom(r), otherIDs(r, clientID), model.ErrorCode_ERROR_CODE_OK, ""
}

// LeaveRoom 主动退出房间，返回剩余房间快照（若房间已销毁则为 nil）与剩余成员 ID
func (m *Manager) LeaveRoom(clientID string) (*model.RoomInfo, []string, model.ErrorCode, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return nil, nil, model.ErrorCode_ERROR_CODE_INTERNAL, "玩家状态不存在"
	}
	if p.RoomCode == "" {
		return nil, nil, model.ErrorCode_ERROR_CODE_NOT_IN_ROOM, "当前不在任何房间中"
	}
	if r, exists := m.rooms[p.RoomCode]; exists && r.Playing {
		return nil, nil, model.ErrorCode_ERROR_CODE_ROOM_PLAYING, "游戏进行中无法主动退出房间"
	}

	roomInfo, notifyIDs := m.leaveRoomLocked(p)
	return roomInfo, notifyIDs, model.ErrorCode_ERROR_CODE_OK, ""
}

// StartGame 由房主发起开始游戏
func (m *Manager) StartGame(clientID string) (string, []*model.PlayerInfo, model.ErrorCode, string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return "", nil, model.ErrorCode_ERROR_CODE_INTERNAL, "玩家状态不存在"
	}
	if p.RoomCode == "" {
		return "", nil, model.ErrorCode_ERROR_CODE_NOT_IN_ROOM, "当前不在任何房间中"
	}

	r, exists := m.rooms[p.RoomCode]
	if !exists {
		return "", nil, model.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND, "房间不存在"
	}
	if r.HostID != clientID {
		return "", nil, model.ErrorCode_ERROR_CODE_NOT_HOST, "只有房主可以开始游戏"
	}
	if r.Playing {
		return "", nil, model.ErrorCode_ERROR_CODE_ROOM_PLAYING, "游戏已经在进行中"
	}
	if len(r.Players) < 2 {
		return "", nil, model.ErrorCode_ERROR_CODE_NOT_ENOUGH_PLAYERS, "至少需要 2 名玩家才能开始游戏"
	}

	r.Playing = true
	r.Offline = make(map[string]bool)

	players := make([]*model.PlayerInfo, len(r.Players))
	for i, member := range r.Players {
		players[i] = &model.PlayerInfo{Id: member.ID, Name: member.Name}
	}
	return r.Code, players, model.ErrorCode_ERROR_CODE_OK, ""
}

// EndGame 游戏结束后恢复房间等待状态，并清理局内已掉线的玩家
func (m *Manager) EndGame(roomCode string) (*model.RoomInfo, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, exists := m.rooms[roomCode]
	if !exists {
		return nil, nil
	}

	r.Playing = false
	onlinePlayers := make([]*Player, 0, len(r.Players))
	for _, p := range r.Players {
		if r.Offline[p.ID] {
			delete(m.players, p.ID)
			continue
		}
		onlinePlayers = append(onlinePlayers, p)
	}
	r.Players = onlinePlayers
	r.Offline = nil

	if len(r.Players) == 0 {
		delete(m.rooms, roomCode)
		return nil, nil
	}

	// 若原房主已掉线被清理，顺延新房主
	hostFound := false
	for _, p := range r.Players {
		if p.ID == r.HostID {
			hostFound = true
			break
		}
	}
	if !hostFound {
		r.HostID = r.Players[0].ID
	}

	return snapshotRoom(r), allIDs(r)
}

// GetPlayerRoom 查询玩家所在房间号及是否正在游戏中
func (m *Manager) GetPlayerRoom(clientID string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.players[clientID]
	if !ok || p.RoomCode == "" {
		return "", false
	}
	r, exists := m.rooms[p.RoomCode]
	if !exists {
		return "", false
	}
	return r.Code, r.Playing
}

// RemovePlayer 玩家掉线清理：等待中自动退房，对局中标记离线（全员离线则销毁房间）
func (m *Manager) RemovePlayer(clientID string) DisconnectResult {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.players[clientID]
	if !ok {
		return DisconnectResult{}
	}

	if p.RoomCode != "" {
		if r, exists := m.rooms[p.RoomCode]; exists && r.Playing {
			if r.Offline == nil {
				r.Offline = make(map[string]bool)
			}
			r.Offline[clientID] = true

			allOffline := true
			for _, member := range r.Players {
				if !r.Offline[member.ID] {
					allOffline = false
					break
				}
			}
			code := r.Code
			if allOffline {
				for _, member := range r.Players {
					delete(m.players, member.ID)
				}
				delete(m.rooms, code)
			}
			return DisconnectResult{
				PlayingRoomCode: code,
				AllOffline:      allOffline,
			}
		}
	}

	var res DisconnectResult
	if p.RoomCode != "" {
		res.RoomInfo, res.NotifyIDs = m.leaveRoomLocked(p)
	}
	delete(m.players, clientID)
	return res
}

func (m *Manager) leaveRoomLocked(p *Player) (*model.RoomInfo, []string) {
	code := p.RoomCode
	p.RoomCode = ""

	r, exists := m.rooms[code]
	if !exists {
		return nil, nil
	}

	for i, member := range r.Players {
		if member.ID == p.ID {
			r.Players = append(r.Players[:i], r.Players[i+1:]...)
			break
		}
	}

	if len(r.Players) == 0 {
		delete(m.rooms, code)
		return nil, nil
	}

	// 若房主离开，顺延给当前第一位成员
	if r.HostID == p.ID {
		r.HostID = r.Players[0].ID
	}

	return snapshotRoom(r), allIDs(r)
}

func (m *Manager) generateUniqueCodeLocked() (string, error) {
	alphabetLen := big.NewInt(int64(len(roomCodeAlphabet)))
	buf := make([]byte, RoomCodeLength)

	for range maxCodeRetries {
		for i := range RoomCodeLength {
			n, err := rand.Int(rand.Reader, alphabetLen)
			if err != nil {
				return "", err
			}
			buf[i] = roomCodeAlphabet[n.Int64()]
		}
		code := string(buf)
		if _, exists := m.rooms[code]; !exists {
			return code, nil
		}
	}
	return "", ErrCodeExhausted
}

// ErrCodeExhausted 极端情况下连续碰撞超过重试次数
var ErrCodeExhausted = &roomError{msg: "failed to generate unique room code"}

type roomError struct{ msg string }

func (e *roomError) Error() string { return e.msg }

func snapshotRoom(r *Room) *model.RoomInfo {
	players := make([]*model.PlayerInfo, len(r.Players))
	for i, p := range r.Players {
		players[i] = &model.PlayerInfo{
			Id:   p.ID,
			Name: p.Name,
		}
	}
	return &model.RoomInfo{
		Code:    r.Code,
		HostId:  r.HostID,
		Players: players,
		Playing: r.Playing,
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
