package game

import (
	"testing"
	"time"

	"github.com/Frankhuex/sanguogao/internal/game/card"
	"github.com/Frankhuex/sanguogao/model"
)

func makeCard(id uint32, t model.CardType) *card.Card {
	return &card.Card{
		ID:       id,
		Type:     t,
		Category: card.CategoryOf(t),
		Suit:     model.CardSuit_CARD_SUIT_SPADE,
		Number:   int32(id%13) + 1,
	}
}

func TestEngineSlashShanNearDeathAndGameOver(t *testing.T) {
	deck := card.NewDeckWithCards([]*card.Card{
		makeCard(1, model.CardType_CARD_TYPE_SHA),
		makeCard(2, model.CardType_CARD_TYPE_SHA),
		makeCard(3, model.CardType_CARD_TYPE_SHAN),
		makeCard(4, model.CardType_CARD_TYPE_TAO),
		makeCard(5, model.CardType_CARD_TYPE_SHA),
		makeCard(6, model.CardType_CARD_TYPE_SHA),
		makeCard(7, model.CardType_CARD_TYPE_SHA),
		makeCard(8, model.CardType_CARD_TYPE_SHAN),
		makeCard(9, model.CardType_CARD_TYPE_SHA),
		makeCard(10, model.CardType_CARD_TYPE_SHA),
	})

	type askEvent struct {
		playerID string
		ask      *model.GameAskPush
	}
	askCh := make(chan askEvent, 16)
	doneCh := make(chan *model.PlayerInfo, 1)

	var seenUseCard, seenDamage, seenDeath bool

	eng := NewEngine(Config{
		RoomCode: "TEST01",
		Players: []*model.PlayerInfo{
			{Id: "p1", Name: "刘备"},
			{Id: "p2", Name: "曹操"},
		},
		InitialHP:    1,
		InitialCards: 2,
		AskTimeout:   2 * time.Second,
		Deck:         deck,
	}, Callbacks{
		SendAsk: func(playerID string, ask *model.GameAskPush) {
			askCh <- askEvent{playerID: playerID, ask: ask}
		},
		Broadcast: func(_ []string, pkt *model.Packet) {
			if ev := pkt.GetGameEventPush(); ev != nil {
				switch ev.Detail.(type) {
				case *model.GameEventPush_UseCard:
					seenUseCard = true
				case *model.GameEventPush_Damage:
					seenDamage = true
				case *model.GameEventPush_Death:
					seenDeath = true
				}
			}
		},
		OnGameOver: func(_ string, winner *model.PlayerInfo) {
			doneCh <- winner
		},
	})

	go eng.Run()

	waitAsk := func(expectPlayer string, expectType model.AskType) *model.GameAskPush {
		t.Helper()
		select {
		case ev := <-askCh:
			if ev.playerID != expectPlayer || ev.ask.GetAskType() != expectType {
				t.Fatalf("expected ask (%s, %v), got (%s, %v)", expectPlayer, expectType, ev.playerID, ev.ask.GetAskType())
			}
			return ev.ask
		case <-time.After(2 * time.Second):
			t.Fatalf("timeout waiting for ask (%s, %v)", expectPlayer, expectType)
			return nil
		}
	}

	// 回合 1：P1 出杀(1)，P2 出闪(3)，P1 结束出牌并弃牌
	ask := waitAsk("p1", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p1", 1, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_USE_CARD,
		CardIds:    []uint32{1},
		TargetId:   "p2",
	})

	ask = waitAsk("p2", model.AskType_ASK_TYPE_RESPOND_SHAN)
	eng.SubmitAction("p2", 2, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_USE_CARD,
		CardIds:    []uint32{3},
	})

	ask = waitAsk("p1", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p1", 3, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})

	ask = waitAsk("p1", model.AskType_ASK_TYPE_DISCARD)
	eng.SubmitAction("p1", 4, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_DISCARD,
		CardIds:    []uint32{5, 6},
	})

	// 回合 2：P2 跳过出牌并弃 7、8，保留桃(4)
	ask = waitAsk("p2", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p2", 5, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})
	ask = waitAsk("p2", model.AskType_ASK_TYPE_DISCARD)
	eng.SubmitAction("p2", 6, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_DISCARD,
		CardIds:    []uint32{7, 8},
	})

	// 回合 3：P1 出杀(2)，P2 放弃出闪 -> 濒死 -> P2 自救桃(4)
	ask = waitAsk("p1", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p1", 7, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_USE_CARD,
		CardIds:    []uint32{2},
		TargetId:   "p2",
	})
	ask = waitAsk("p2", model.AskType_ASK_TYPE_RESPOND_SHAN)
	eng.SubmitAction("p2", 8, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})
	ask = waitAsk("p1", model.AskType_ASK_TYPE_SAVE_TAO)
	eng.SubmitAction("p1", 9, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})
	ask = waitAsk("p2", model.AskType_ASK_TYPE_SAVE_TAO)
	eng.SubmitAction("p2", 10, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_USE_CARD,
		CardIds:    []uint32{4},
	})

	ask = waitAsk("p1", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p1", 11, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})
	ask = waitAsk("p1", model.AskType_ASK_TYPE_DISCARD)
	eng.SubmitAction("p1", 12, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_DISCARD,
		CardIds:    []uint32{10},
	})

	// 回合 4：P2 触发重洗并掉线托管
	ask = waitAsk("p2", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p2", 13, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})
	eng.MarkOffline("p2")

	// 回合 5：P1 击杀 P2 触发 GameOver
	ask = waitAsk("p1", model.AskType_ASK_TYPE_PLAY)
	eng.SubmitAction("p1", 14, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_USE_CARD,
		CardIds:    []uint32{9},
		TargetId:   "p2",
	})
	ask = waitAsk("p1", model.AskType_ASK_TYPE_SAVE_TAO)
	eng.SubmitAction("p1", 15, &model.GameActionReq{
		AskId:      ask.AskId,
		ActionType: model.ActionType_ACTION_TYPE_PASS,
	})

	select {
	case winner := <-doneCh:
		if winner == nil || winner.Id != "p1" {
			t.Fatalf("expected winner p1, got %+v", winner)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for GameOver")
	}

	if !seenUseCard || !seenDamage || !seenDeath {
		t.Fatalf("expected structured events (useCard=%v, damage=%v, death=%v)", seenUseCard, seenDamage, seenDeath)
	}
}

func TestEngineAllOfflineStopsCleanly(t *testing.T) {
	stoppedCh := make(chan struct{}, 1)
	eng := NewEngine(Config{
		RoomCode: "TEST02",
		Players: []*model.PlayerInfo{
			{Id: "p1", Name: "A"},
			{Id: "p2", Name: "B"},
		},
		AskTimeout: 5 * time.Second,
	}, Callbacks{
		OnStopped: func(_ string) {
			stoppedCh <- struct{}{}
		},
	})

	go eng.Run()
	eng.MarkOffline("p1")
	eng.MarkOffline("p2")

	select {
	case <-stoppedCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected engine to stop when all players offline")
	}
}
