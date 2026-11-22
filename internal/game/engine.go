package game

import (
	"fmt"
	"sync"
	"time"

	"github.com/Frankhuex/sanguogao/internal/game/card"
	"github.com/Frankhuex/sanguogao/model"
)

const (
	DefaultInitialHP    = 4
	DefaultInitialCards = 4
	DefaultAskTimeout   = 20 * time.Second
)

type Config struct {
	RoomCode     string
	Players      []*model.PlayerInfo
	InitialHP    int32
	InitialCards int
	AskTimeout   time.Duration
	Deck         *card.Deck   // 可选：测试时注入确定性牌堆
	WinCondition WinCondition // 可选：默认 LastStandingWinCondition
}

type Callbacks struct {
	SendState      func(playerID string, state *model.GameStatePush)
	SendAsk        func(playerID string, ask *model.GameAskPush)
	SendActionResp func(playerID string, seq uint64, code model.ErrorCode, msg string)
	Broadcast      func(playerIDs []string, pkt *model.Packet)
	OnGameOver     func(roomCode string, winner *model.PlayerInfo)
	OnStopped      func(roomCode string)
}

type IncomingAction struct {
	ClientID string
	Seq      uint64
	Req      *model.GameActionReq
}

type errGameOver struct {
	winner *GamePlayer
}

type errGameStopped struct{}

type Engine struct {
	roomCode     string
	players      []*GamePlayer
	playerByID   map[string]*GamePlayer
	deck         *card.Deck
	winCond      WinCondition
	initialCards int
	askTimeout   time.Duration

	currentSeat int
	extraTurns  []*GamePlayer
	currentTurn *TurnContext
	nextAskID   uint64

	actionCh  chan IncomingAction
	offlineCh chan string
	stopCh    chan struct{}
	stopOnce  sync.Once

	cb Callbacks
}

func NewEngine(cfg Config, cb Callbacks) *Engine {
	initialHP := cfg.InitialHP
	if initialHP <= 0 {
		initialHP = DefaultInitialHP
	}
	initialCards := cfg.InitialCards
	if initialCards <= 0 {
		initialCards = DefaultInitialCards
	}
	askTimeout := cfg.AskTimeout
	if askTimeout <= 0 {
		askTimeout = DefaultAskTimeout
	}
	deck := cfg.Deck
	if deck == nil {
		deck = card.NewDefaultDeck()
	}
	winCond := cfg.WinCondition
	if winCond == nil {
		winCond = LastStandingWinCondition{}
	}

	players := make([]*GamePlayer, len(cfg.Players))
	byID := make(map[string]*GamePlayer, len(cfg.Players))
	for i, info := range cfg.Players {
		gp := &GamePlayer{
			ID:    info.GetId(),
			Name:  info.GetName(),
			Seat:  i,
			HP:    initialHP,
			MaxHP: initialHP,
			Alive: true,
		}
		players[i] = gp
		byID[gp.ID] = gp
	}

	return &Engine{
		roomCode:     cfg.RoomCode,
		players:      players,
		playerByID:   byID,
		deck:         deck,
		winCond:      winCond,
		initialCards: initialCards,
		askTimeout:   askTimeout,
		currentSeat:  -1,
		actionCh:     make(chan IncomingAction, 32),
		offlineCh:    make(chan string, len(players)+4),
		stopCh:       make(chan struct{}),
		cb:           cb,
	}
}

// SubmitAction 由外部 handler 调用，投递玩家操作到游戏协程
func (g *Engine) SubmitAction(clientID string, seq uint64, req *model.GameActionReq) {
	select {
	case <-g.stopCh:
		g.sendActionResp(clientID, seq, model.ErrorCode_ERROR_CODE_INVALID_ACTION, "游戏已结束")
	case g.actionCh <- IncomingAction{ClientID: clientID, Seq: seq, Req: req}:
	default:
		g.sendActionResp(clientID, seq, model.ErrorCode_ERROR_CODE_INTERNAL, "操作过于频繁，请稍后重试")
	}
}

// MarkOffline 标记玩家掉线（进入自动托管）
func (g *Engine) MarkOffline(clientID string) {
	select {
	case <-g.stopCh:
	case g.offlineCh <- clientID:
	default:
	}
}

// Stop 强制停止游戏协程
func (g *Engine) Stop() {
	g.stopOnce.Do(func() {
		close(g.stopCh)
	})
}

// Run 游戏主循环（应在独立 goroutine 中运行）
func (g *Engine) Run() {
	defer func() {
		r := recover()
		g.Stop()
		if r == nil {
			return
		}
		switch v := r.(type) {
		case errGameOver:
			g.pushStateToAll("")
			var winnerInfo *model.PlayerInfo
			var winnerID, winnerName string
			if v.winner != nil {
				winnerID = v.winner.ID
				winnerName = v.winner.Name
				winnerInfo = &model.PlayerInfo{Id: winnerID, Name: winnerName}
				g.broadcastTextEvent(fmt.Sprintf("游戏结束！%s 获得胜利！", winnerName))
			} else {
				g.broadcastTextEvent("游戏结束！平局！")
			}
			g.broadcast(&model.Packet{
				Seq: 0,
				Payload: &model.Packet_GameOverPush{
					GameOverPush: &model.GameOverPush{
						WinnerId:   winnerID,
						WinnerName: winnerName,
					},
				},
			})
			if g.cb.OnGameOver != nil {
				g.cb.OnGameOver(g.roomCode, winnerInfo)
			}
		case errGameStopped:
			if g.cb.OnStopped != nil {
				g.cb.OnStopped(g.roomCode)
			}
		default:
			panic(r)
		}
	}()

	g.trigger(TimingAfterRevealHero, nil)
	g.dealInitialCards()
	g.trigger(TimingGameStart, nil)
	g.broadcastTextEvent("游戏开始！")
	g.pushStateToAll("")
	g.checkWinCondition()

	for {
		p := g.nextTurnPlayer()
		if p == nil {
			panic(errGameOver{winner: nil})
		}
		g.runTurn(p)
	}
}

func (g *Engine) dealInitialCards() {
	for _, p := range g.players {
		p.Hand = append(p.Hand, g.deck.Draw(g.initialCards)...)
	}
}

func (g *Engine) nextTurnPlayer() *GamePlayer {
	for len(g.extraTurns) > 0 {
		p := g.extraTurns[0]
		g.extraTurns = g.extraTurns[1:]
		if p.Alive {
			return p
		}
	}

	n := len(g.players)
	for range n {
		g.currentSeat = (g.currentSeat + 1) % n
		if g.players[g.currentSeat].Alive {
			return g.players[g.currentSeat]
		}
	}
	return nil
}

func (g *Engine) runTurn(p *GamePlayer) {
	g.trigger(TimingBeforeTurnStart, p)

	turn := NewTurnContext(p)
	g.currentTurn = turn
	p.SlashUsed = 0

	g.trigger(TimingTurnStart, turn)
	g.broadcastEvent(&model.GameEventPush{
		Text: fmt.Sprintf("轮到 %s 的回合", p.Name),
		Detail: &model.GameEventPush_TurnStart{
			TurnStart: &model.TurnStartEvent{PlayerId: p.ID},
		},
	})

	for _, phase := range turn.Phases {
		if !p.Alive {
			break
		}
		if turn.IsSkipped(phase) {
			continue
		}
		turn.CurrentPhase = phase
		g.trigger(TimingPhaseStart, turn)
		if !turn.IsSkipped(phase) && p.Alive {
			g.execPhase(phase, turn)
		}
		g.trigger(TimingPhaseEnd, turn)
		g.trigger(TimingPhaseGap, turn)
	}

	g.trigger(TimingTurnEnd, turn)
	g.trigger(TimingAfterTurnEnd, turn)
	g.currentTurn = nil
}

// askPlayer 向目标玩家发起阻塞询问，支持校验、超时托管、掉线托管
func (g *Engine) askPlayer(target *GamePlayer, ask *model.GameAskPush, validate func(*model.GameActionReq) string) *model.GameActionReq {
	g.drainOffline()
	if g.allAliveOffline() {
		panic(errGameStopped{})
	}

	g.nextAskID++
	askID := g.nextAskID
	ask.AskId = askID
	ask.TimeoutSec = int32(g.askTimeout / time.Second)
	if ask.TimeoutSec <= 0 {
		ask.TimeoutSec = 1
	}

	g.pushStateToAll(target.ID)

	if target.Offline {
		return g.defaultAction(target, ask)
	}

	if g.cb.SendAsk != nil {
		g.cb.SendAsk(target.ID, ask)
	}

	timer := time.NewTimer(g.askTimeout)
	defer timer.Stop()

	for {
		select {
		case <-g.stopCh:
			panic(errGameStopped{})

		case offID := <-g.offlineCh:
			g.applyOffline(offID)
			if g.allAliveOffline() {
				panic(errGameStopped{})
			}
			if offID == target.ID {
				return g.defaultAction(target, ask)
			}

		case <-timer.C:
			return g.defaultAction(target, ask)

		case act := <-g.actionCh:
			if act.ClientID != target.ID || act.Req.GetAskId() != askID {
				g.sendActionResp(act.ClientID, act.Seq, model.ErrorCode_ERROR_CODE_INVALID_ACTION, "当前无需你操作或请求已过期")
				continue
			}
			if errMsg := validate(act.Req); errMsg != "" {
				g.sendActionResp(act.ClientID, act.Seq, model.ErrorCode_ERROR_CODE_INVALID_ACTION, errMsg)
				continue
			}
			g.sendActionResp(act.ClientID, act.Seq, model.ErrorCode_ERROR_CODE_OK, "")
			return act.Req
		}
	}
}

func (g *Engine) defaultAction(target *GamePlayer, ask *model.GameAskPush) *model.GameActionReq {
	if ask.GetAskType() == model.AskType_ASK_TYPE_DISCARD {
		n := int(ask.GetDiscardCount())
		if n > len(target.Hand) {
			n = len(target.Hand)
		}
		ids := make([]uint32, 0, n)
		start := len(target.Hand) - n
		for i := start; i < len(target.Hand); i++ {
			ids = append(ids, target.Hand[i].ID)
		}
		return &model.GameActionReq{
			AskId:      ask.GetAskId(),
			ActionType: model.ActionType_ACTION_TYPE_DISCARD,
			CardIds:    ids,
		}
	}
	return &model.GameActionReq{
		AskId:      ask.GetAskId(),
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	}
}

func (g *Engine) drainOffline() {
	for {
		select {
		case offID := <-g.offlineCh:
			g.applyOffline(offID)
		default:
			return
		}
	}
}

func (g *Engine) applyOffline(clientID string) {
	if p, ok := g.playerByID[clientID]; ok && !p.Offline {
		p.Offline = true
		g.broadcastTextEvent(fmt.Sprintf("%s 已掉线，进入自动托管", p.Name))
	}
}

func (g *Engine) allAliveOffline() bool {
	for _, p := range g.players {
		if p.Alive && !p.Offline {
			return false
		}
	}
	return true
}

func (g *Engine) checkWinCondition() {
	if over, winner := g.winCond.Check(g.players); over {
		panic(errGameOver{winner: winner})
	}
}

// ===== 数值修正与时机钩子（预留给技能/装备系统） =====

func (g *Engine) trigger(_ Timing, _ any) {}

func (g *Engine) getDrawCount(_ *GamePlayer) int { return 2 }

func (g *Engine) getSlashLimit(_ *GamePlayer) int { return 1 }

func (g *Engine) getMaxHandCards(p *GamePlayer) int {
	if p.HP < 0 {
		return 0
	}
	return int(p.HP)
}

// ===== 状态与事件广播 =====

func (g *Engine) pushStateToAll(waitingPlayerID string) {
	if g.cb.SendState == nil {
		return
	}
	publicPlayers := make([]*model.GamePlayerPublic, len(g.players))
	for i, p := range g.players {
		publicPlayers[i] = p.ToPublicProto()
	}

	var curTurnID string
	var curPhase model.Phase
	if g.currentTurn != nil {
		curTurnID = g.currentTurn.Player.ID
		curPhase = g.currentTurn.CurrentPhase
	}

	for _, p := range g.players {
		if p.Offline {
			continue
		}
		g.cb.SendState(p.ID, &model.GameStatePush{
			RoomCode:            g.roomCode,
			CurrentTurnPlayerId: curTurnID,
			Phase:               curPhase,
			DrawPileCount:       int32(g.deck.DrawCount()),
			DiscardPileCount:    int32(g.deck.DiscardCount()),
			Players:             publicPlayers,
			SelfHand:            p.HandToProto(),
			WaitingPlayerId:     waitingPlayerID,
		})
	}
}

func (g *Engine) broadcastTextEvent(text string) {
	g.broadcastEvent(&model.GameEventPush{Text: text})
}

func (g *Engine) broadcastEvent(ev *model.GameEventPush) {
	g.broadcast(&model.Packet{
		Seq: 0,
		Payload: &model.Packet_GameEventPush{
			GameEventPush: ev,
		},
	})
}

func (g *Engine) broadcast(pkt *model.Packet) {
	if g.cb.Broadcast == nil {
		return
	}
	ids := make([]string, 0, len(g.players))
	for _, p := range g.players {
		if !p.Offline {
			ids = append(ids, p.ID)
		}
	}
	g.cb.Broadcast(ids, pkt)
}

func (g *Engine) sendActionResp(clientID string, seq uint64, code model.ErrorCode, msg string) {
	if g.cb.SendActionResp != nil {
		g.cb.SendActionResp(clientID, seq, code, msg)
	}
}
