package handler

import "github.com/Frankhuex/sanguogao/model"

func (h *Handler) handleSetName(clientID string, seq uint64, req *model.SetNameReq) {
	self, roomInfo, notifyIDs, code, msg := h.roomMgr.SetName(clientID, req.GetName())
	h.sendPacket(clientID, &model.Packet{
		Seq: seq,
		Payload: &model.Packet_SetNameResp{
			SetNameResp: &model.SetNameResp{
				Code:    code,
				Message: msg,
				Self:    self,
			},
		},
	})
	if code == model.ErrorCode_ERROR_CODE_OK {
		h.pushRoomState(notifyIDs, roomInfo)
	}
}

func (h *Handler) handleCreateRoom(clientID string, seq uint64) {
	roomInfo, code, msg := h.roomMgr.CreateRoom(clientID)
	h.sendPacket(clientID, &model.Packet{
		Seq: seq,
		Payload: &model.Packet_CreateRoomResp{
			CreateRoomResp: &model.CreateRoomResp{
				Code:    code,
				Message: msg,
				Room:    roomInfo,
			},
		},
	})
}

func (h *Handler) handleJoinRoom(clientID string, seq uint64, req *model.JoinRoomReq) {
	roomInfo, notifyIDs, code, msg := h.roomMgr.JoinRoom(clientID, req.GetRoomCode())
	h.sendPacket(clientID, &model.Packet{
		Seq: seq,
		Payload: &model.Packet_JoinRoomResp{
			JoinRoomResp: &model.JoinRoomResp{
				Code:    code,
				Message: msg,
				Room:    roomInfo,
			},
		},
	})
	if code == model.ErrorCode_ERROR_CODE_OK {
		h.pushRoomState(notifyIDs, roomInfo)
	}
}

func (h *Handler) handleLeaveRoom(clientID string, seq uint64) {
	roomInfo, notifyIDs, code, msg := h.roomMgr.LeaveRoom(clientID)
	h.sendPacket(clientID, &model.Packet{
		Seq: seq,
		Payload: &model.Packet_LeaveRoomResp{
			LeaveRoomResp: &model.LeaveRoomResp{
				Code:    code,
				Message: msg,
			},
		},
	})
	if code == model.ErrorCode_ERROR_CODE_OK {
		h.pushRoomState(notifyIDs, roomInfo)
	}
}

func (h *Handler) pushRoomState(clientIDs []string, roomInfo *model.RoomInfo) {
	if roomInfo == nil || len(clientIDs) == 0 {
		return
	}
	h.broadcastPacket(clientIDs, &model.Packet{
		Seq: 0,
		Payload: &model.Packet_RoomStatePush{
			RoomStatePush: &model.RoomStatePush{
				Room: roomInfo,
			},
		},
	})
}
