package handler

import (
	"github.com/Frankhuex/sanguogao/internal/game"
	"github.com/Frankhuex/sanguogao/model"
)

func (h *Handler) handleStartGame(clientID string, seq uint64) {
	roomCode, players, code, msg := h.roomMgr.StartGame(clientID)
	h.sendPacket(clientID, &model.Packet{
		Seq: seq,
		Payload: &model.Packet_StartGameResp{
			StartGameResp: &model.StartGameResp{
				Code:    code,
				Message: msg,
			},
		},
	})
	if code != model.ErrorCode_ERROR_CODE_OK {
		return
	}

	var eng *game.Engine
	eng = game.NewEngine(game.Config{
		RoomCode: roomCode,
		Players:  players,
	}, game.Callbacks{
		SendState: func(playerID string, state *model.GameStatePush) {
			h.sendPacket(playerID, &model.Packet{
				Seq:     0,
				Payload: &model.Packet_GameStatePush{GameStatePush: state},
			})
		},
		SendAsk: func(playerID string, ask *model.GameAskPush) {
			h.sendPacket(playerID, &model.Packet{
				Seq:     0,
				Payload: &model.Packet_GameAskPush{GameAskPush: ask},
			})
		},
		SendActionResp: func(playerID string, reqSeq uint64, errCode model.ErrorCode, errMsg string) {
			h.sendPacket(playerID, &model.Packet{
				Seq: reqSeq,
				Payload: &model.Packet_GameActionResp{
					GameActionResp: &model.GameActionResp{
						Code:    errCode,
						Message: errMsg,
					},
				},
			})
		},
		Broadcast: func(playerIDs []string, pkt *model.Packet) {
			h.broadcastPacket(playerIDs, pkt)
		},
		OnGameOver: func(code string, _ *model.PlayerInfo) {
			h.deleteGame(code, eng)
			roomInfo, onlineIDs := h.roomMgr.EndGame(code)
			h.pushRoomState(onlineIDs, roomInfo)
		},
		OnStopped: func(code string) {
			h.deleteGame(code, eng)
			h.roomMgr.EndGame(code)
		},
	})

	h.gamesMu.Lock()
	h.games[roomCode] = eng
	h.gamesMu.Unlock()

	go eng.Run()
}

func (h *Handler) handleGameAction(clientID string, seq uint64, req *model.GameActionReq) {
	roomCode, playing := h.roomMgr.GetPlayerRoom(clientID)
	if !playing || roomCode == "" {
		h.sendPacket(clientID, &model.Packet{
			Seq: seq,
			Payload: &model.Packet_GameActionResp{
				GameActionResp: &model.GameActionResp{
					Code:    model.ErrorCode_ERROR_CODE_INVALID_ACTION,
					Message: "当前不在对局中",
				},
			},
		})
		return
	}

	eng := h.getGame(roomCode)
	if eng == nil {
		h.sendPacket(clientID, &model.Packet{
			Seq: seq,
			Payload: &model.Packet_GameActionResp{
				GameActionResp: &model.GameActionResp{
					Code:    model.ErrorCode_ERROR_CODE_INVALID_ACTION,
					Message: "对局已结束",
				},
			},
		})
		return
	}

	eng.SubmitAction(clientID, seq, req)
}

func (h *Handler) getGame(roomCode string) *game.Engine {
	h.gamesMu.RLock()
	eng := h.games[roomCode]
	h.gamesMu.RUnlock()
	return eng
}

func (h *Handler) deleteGame(roomCode string, eng *game.Engine) {
	h.gamesMu.Lock()
	if cur, ok := h.games[roomCode]; ok && cur == eng {
		delete(h.games, roomCode)
	}
	h.gamesMu.Unlock()
}
