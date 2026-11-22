package game

import (
	"fmt"
	"strings"

	"github.com/Frankhuex/sanguogao/internal/game/card"
	"github.com/Frankhuex/sanguogao/model"
)

func (g *Engine) execPhase(phase model.Phase, turn *TurnContext) {
	p := turn.Player
	switch phase {
	case model.Phase_PHASE_PREPARE:
		// 预留给准备阶段技能
	case model.Phase_PHASE_JUDGE:
		// 预留给延时锦囊判定循环
	case model.Phase_PHASE_DRAW:
		count := g.getDrawCount(p)
		drawn := g.deck.Draw(count)
		p.Hand = append(p.Hand, drawn...)
		g.broadcastEvent(&model.GameEventPush{
			Text: fmt.Sprintf("%s 摸了 %d 张牌", p.Name, len(drawn)),
			Detail: &model.GameEventPush_DrawCards{
				DrawCards: &model.DrawCardsEvent{
					PlayerId: p.ID,
					Count:    int32(len(drawn)),
				},
			},
		})
		g.pushStateToAll("")
	case model.Phase_PHASE_PLAY:
		g.execPlayPhase(p)
	case model.Phase_PHASE_DISCARD:
		g.execDiscardPhase(p)
	case model.Phase_PHASE_FINISH:
		// 预留给结束阶段技能
	}
}

func (g *Engine) execPlayPhase(p *GamePlayer) {
	for p.Alive {
		slashRemaining := g.getSlashLimit(p) - p.SlashUsed
		if slashRemaining < 0 {
			slashRemaining = 0
		}

		ask := &model.GameAskPush{
			AskType:        model.AskType_ASK_TYPE_PLAY,
			SlashRemaining: int32(slashRemaining),
		}

		req := g.askPlayer(p, ask, func(r *model.GameActionReq) string {
			switch r.GetActionType() {
			case model.ActionType_ACTION_TYPE_PASS:
				return ""
			case model.ActionType_ACTION_TYPE_USE_CARD:
				ids := r.GetCardIds()
				if len(ids) != 1 {
					return "每次只能使用 1 张牌"
				}
				c, _ := p.FindCard(ids[0])
				if c == nil {
					return "手牌中不存在该卡牌"
				}
				switch c.Type {
				case model.CardType_CARD_TYPE_SHA:
					if p.SlashUsed >= g.getSlashLimit(p) {
						return "本回合出【杀】次数已用完"
					}
					target := g.playerByID[r.GetTargetId()]
					if target == nil || !target.Alive {
						return "目标不存在或已阵亡"
					}
					if target.ID == p.ID {
						return "不能对自己使用【杀】"
					}
					return ""
				case model.CardType_CARD_TYPE_TAO:
					if p.HP >= p.MaxHP {
						return "体力已满，不能使用【桃】"
					}
					return ""
				default:
					return "出牌阶段不能主动使用【闪】"
				}
			default:
				return "不支持的操作类型"
			}
		})

		if req.GetActionType() == model.ActionType_ACTION_TYPE_PASS {
			g.broadcastTextEvent(fmt.Sprintf("%s 结束了出牌阶段", p.Name))
			return
		}

		cardID := req.GetCardIds()[0]
		used, ok := p.RemoveCardByID(cardID)
		if !ok {
			continue
		}
		g.deck.Discard(used)

		switch used.Type {
		case model.CardType_CARD_TYPE_SHA:
			p.SlashUsed++
			target := g.playerByID[req.GetTargetId()]
			g.broadcastEvent(&model.GameEventPush{
				Text: fmt.Sprintf("%s 对 %s 使用了【杀】", p.Name, target.Name),
				Detail: &model.GameEventPush_UseCard{
					UseCard: &model.UseCardEvent{
						SourceId: p.ID,
						TargetId: target.ID,
						Card:     used.ToProto(),
					},
				},
			})
			g.resolveUseSlash(p, target)
		case model.CardType_CARD_TYPE_TAO:
			p.HP++
			g.broadcastEvent(&model.GameEventPush{
				Text: fmt.Sprintf("%s 使用了【桃】，回复 1 点体力（%d/%d）", p.Name, p.HP, p.MaxHP),
				Detail: &model.GameEventPush_Recover{
					Recover: &model.RecoverEvent{
						TargetId: p.ID,
						Amount:   1,
					},
				},
			})
			g.pushStateToAll("")
		}
	}
}

func (g *Engine) resolveUseSlash(src, dst *GamePlayer) {
	ask := &model.GameAskPush{
		AskType:        model.AskType_ASK_TYPE_RESPOND_SHAN,
		SourcePlayerId: src.ID,
	}

	req := g.askPlayer(dst, ask, func(r *model.GameActionReq) string {
		switch r.GetActionType() {
		case model.ActionType_ACTION_TYPE_PASS:
			return ""
		case model.ActionType_ACTION_TYPE_USE_CARD:
			ids := r.GetCardIds()
			if len(ids) != 1 {
				return "请使用 1 张【闪】"
			}
			c, _ := dst.FindCard(ids[0])
			if c == nil || c.Type != model.CardType_CARD_TYPE_SHAN {
				return "必须使用手牌中的【闪】"
			}
			return ""
		default:
			return "请打出【闪】或放弃"
		}
	})

	if req.GetActionType() == model.ActionType_ACTION_TYPE_USE_CARD {
		if shan, ok := dst.RemoveCardByID(req.GetCardIds()[0]); ok {
			g.deck.Discard(shan)
			g.broadcastEvent(&model.GameEventPush{
				Text: fmt.Sprintf("%s 打出【闪】，抵消了【杀】", dst.Name),
				Detail: &model.GameEventPush_UseCard{
					UseCard: &model.UseCardEvent{
						SourceId: dst.ID,
						Card:     shan.ToProto(),
					},
				},
			})
			g.pushStateToAll("")
			return
		}
	}

	g.applyDamage(src, dst, 1, model.DamageElement_DAMAGE_ELEMENT_NORMAL)
}

func (g *Engine) applyDamage(src, dst *GamePlayer, amount int32, element model.DamageElement) {
	dst.HP -= amount
	g.trigger(TimingDamage, dst)

	srcID := ""
	srcName := "系统"
	if src != nil {
		srcID = src.ID
		srcName = src.Name
	}
	g.broadcastEvent(&model.GameEventPush{
		Text: fmt.Sprintf("%s 受到 %s 造成的 %d 点伤害（当前体力 %d/%d）", dst.Name, srcName, amount, dst.HP, dst.MaxHP),
		Detail: &model.GameEventPush_Damage{
			Damage: &model.DamageEvent{
				SourceId: srcID,
				TargetId: dst.ID,
				Amount:   amount,
				Element:  element,
			},
		},
	})
	g.pushStateToAll("")

	if dst.HP <= 0 {
		g.resolveNearDeath(dst)
	}
}

func (g *Engine) resolveNearDeath(target *GamePlayer) {
	g.trigger(TimingNearDeath, target)
	g.broadcastEvent(&model.GameEventPush{
		Text: fmt.Sprintf("%s 进入濒死状态！", target.Name),
		Detail: &model.GameEventPush_NearDeath{
			NearDeath: &model.NearDeathEvent{TargetId: target.ID},
		},
	})

	startSeat := target.Seat
	if g.currentTurn != nil && g.currentTurn.Player != nil {
		startSeat = g.currentTurn.Player.Seat
	}

	n := len(g.players)
	for offset := range n {
		rescuer := g.players[(startSeat+offset)%n]
		if !rescuer.Alive {
			continue
		}

		for target.HP <= 0 {
			ask := &model.GameAskPush{
				AskType:        model.AskType_ASK_TYPE_SAVE_TAO,
				SourcePlayerId: target.ID,
			}

			req := g.askPlayer(rescuer, ask, func(r *model.GameActionReq) string {
				switch r.GetActionType() {
				case model.ActionType_ACTION_TYPE_PASS:
					return ""
				case model.ActionType_ACTION_TYPE_USE_CARD:
					ids := r.GetCardIds()
					if len(ids) != 1 {
						return "请使用 1 张【桃】"
					}
					c, _ := rescuer.FindCard(ids[0])
					if c == nil || c.Type != model.CardType_CARD_TYPE_TAO {
						return "必须使用手牌中的【桃】"
					}
					return ""
				default:
					return "请使用【桃】或放弃"
				}
			})

			if req.GetActionType() == model.ActionType_ACTION_TYPE_PASS {
				break
			}

			if tao, ok := rescuer.RemoveCardByID(req.GetCardIds()[0]); ok {
				g.deck.Discard(tao)
				target.HP++
				g.broadcastEvent(&model.GameEventPush{
					Text: fmt.Sprintf("%s 对 %s 使用了【桃】，回复 1 点体力（%d/%d）", rescuer.Name, target.Name, target.HP, target.MaxHP),
					Detail: &model.GameEventPush_Recover{
						Recover: &model.RecoverEvent{
							TargetId: target.ID,
							Amount:   1,
						},
					},
				})
				g.pushStateToAll("")
			}
		}

		if target.HP > 0 {
			g.broadcastTextEvent(fmt.Sprintf("%s 脱离濒死状态", target.Name))
			return
		}
	}

	if target.HP <= 0 {
		g.handleDeath(target)
	}
}

func (g *Engine) handleDeath(target *GamePlayer) {
	target.Alive = false
	if len(target.Hand) > 0 {
		g.deck.Discard(target.Hand...)
		target.Hand = nil
	}
	g.trigger(TimingDeath, target)
	g.broadcastEvent(&model.GameEventPush{
		Text: fmt.Sprintf("%s 阵亡了！", target.Name),
		Detail: &model.GameEventPush_Death{
			Death: &model.DeathEvent{TargetId: target.ID},
		},
	})
	g.pushStateToAll("")
	g.checkWinCondition()
}

func (g *Engine) execDiscardPhase(p *GamePlayer) {
	maxHand := g.getMaxHandCards(p)
	excess := len(p.Hand) - maxHand
	if excess <= 0 {
		return
	}

	ask := &model.GameAskPush{
		AskType:      model.AskType_ASK_TYPE_DISCARD,
		DiscardCount: int32(excess),
	}

	req := g.askPlayer(p, ask, func(r *model.GameActionReq) string {
		if r.GetActionType() != model.ActionType_ACTION_TYPE_DISCARD {
			return fmt.Sprintf("请选择 %d 张手牌弃置", excess)
		}
		ids := r.GetCardIds()
		if len(ids) != excess {
			return fmt.Sprintf("必须恰好弃置 %d 张手牌", excess)
		}
		seen := make(map[uint32]struct{}, len(ids))
		for _, id := range ids {
			if _, dup := seen[id]; dup {
				return "不能重复选择同一张牌"
			}
			if c, _ := p.FindCard(id); c == nil {
				return "包含不在手牌中的卡牌"
			}
			seen[id] = struct{}{}
		}
		return ""
	})

	discarded := make([]*card.Card, 0, excess)
	discardedNames := make([]string, 0, excess)
	for _, id := range req.GetCardIds() {
		if c, ok := p.RemoveCardByID(id); ok {
			g.deck.Discard(c)
			discarded = append(discarded, c)
			discardedNames = append(discardedNames, "【"+card.TypeName(c.Type)+"】")
		}
	}
	g.broadcastEvent(&model.GameEventPush{
		Text: fmt.Sprintf("%s 弃置了 %s", p.Name, strings.Join(discardedNames, "、")),
		Detail: &model.GameEventPush_DiscardCards{
			DiscardCards: &model.DiscardCardsEvent{
				PlayerId: p.ID,
				Cards:    card.ToProtoSlice(discarded),
			},
		},
	})
	g.pushStateToAll("")
}
