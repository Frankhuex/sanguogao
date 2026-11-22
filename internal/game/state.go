package game

import (
	"github.com/Frankhuex/sanguogao/internal/game/card"
	"github.com/Frankhuex/sanguogao/model"
)

var defaultPhases = []model.Phase{
	model.Phase_PHASE_PREPARE,
	model.Phase_PHASE_JUDGE,
	model.Phase_PHASE_DRAW,
	model.Phase_PHASE_PLAY,
	model.Phase_PHASE_DISCARD,
	model.Phase_PHASE_FINISH,
}

// Timing 规则集 3.0 的关键时机点（预留给技能与装备触发器）
type Timing int

const (
	TimingAfterRevealHero Timing = iota + 1
	TimingGameStart
	TimingBeforeTurnStart
	TimingTurnStart
	TimingPhaseStart
	TimingPhaseEnd
	TimingPhaseGap
	TimingTurnEnd
	TimingAfterTurnEnd
	TimingDamage
	TimingNearDeath
	TimingDeath
)

type TurnContext struct {
	Player       *GamePlayer
	Phases       []model.Phase
	CurrentPhase model.Phase
	Skipped      map[model.Phase]bool
}

func NewTurnContext(p *GamePlayer) *TurnContext {
	phases := make([]model.Phase, len(defaultPhases))
	copy(phases, defaultPhases)
	return &TurnContext{
		Player:  p,
		Phases:  phases,
		Skipped: make(map[model.Phase]bool),
	}
}

func (t *TurnContext) IsSkipped(ph model.Phase) bool {
	return t.Skipped[ph]
}

func (t *TurnContext) Skip(ph model.Phase) {
	t.Skipped[ph] = true
}

type GamePlayer struct {
	ID        string
	Name      string
	Seat      int
	HP        int32
	MaxHP     int32
	Hand      []*card.Card
	Alive     bool
	Offline   bool
	SlashUsed int
}

func (p *GamePlayer) FindCard(cardID uint32) (*card.Card, int) {
	for i, c := range p.Hand {
		if c.ID == cardID {
			return c, i
		}
	}
	return nil, -1
}

func (p *GamePlayer) RemoveCardByID(cardID uint32) (*card.Card, bool) {
	c, idx := p.FindCard(cardID)
	if idx < 0 {
		return nil, false
	}
	p.Hand = append(p.Hand[:idx], p.Hand[idx+1:]...)
	return c, true
}

func (p *GamePlayer) ToPublicProto() *model.GamePlayerPublic {
	return &model.GamePlayerPublic{
		Id:        p.ID,
		Name:      p.Name,
		Seat:      int32(p.Seat),
		Hp:        p.HP,
		MaxHp:     p.MaxHP,
		HandCount: int32(len(p.Hand)),
		Alive:     p.Alive,
		Offline:   p.Offline,
	}
}

func (p *GamePlayer) HandToProto() []*model.CardInfo {
	return card.ToProtoSlice(p.Hand)
}
