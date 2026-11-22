package handler

import (
	"log"

	"github.com/Frankhuex/sanguogao/model"
	"github.com/Frankhuex/sanguogao/room"
	"github.com/Frankhuex/sanguogao/ws"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	wsMgr   *ws.Manager
	roomMgr *room.Manager
}

func New(wsMgr *ws.Manager, roomMgr *room.Manager) *Handler {
	return &Handler{
		wsMgr:   wsMgr,
		roomMgr: roomMgr,
	}
}

// OnConnect 新连接建立：注册玩家并推送 WelcomePush
func (h *Handler) OnConnect(clientID string) {
	self := h.roomMgr.AddPlayer(clientID)
	h.sendPacket(clientID, &model.Packet{
		Seq: 0,
		Payload: &model.Packet_WelcomePush{
			WelcomePush: &model.WelcomePush{
				Self: self,
			},
		},
	})
}

// OnDisconnect 连接断开：自动退房并通知剩余成员
func (h *Handler) OnDisconnect(clientID string) {
	roomInfo, notifyIDs := h.roomMgr.RemovePlayer(clientID)
	h.pushRoomState(notifyIDs, roomInfo)
}

// OnMessage 路由并处理客户端发来的 Protobuf 消息
func (h *Handler) OnMessage(clientID string, data []byte) {
	var pkt model.Packet
	if err := proto.Unmarshal(data, &pkt); err != nil {
		log.Printf("[handler] unmarshal error from %s: %v", clientID, err)
		return
	}

	switch p := pkt.Payload.(type) {
	case *model.Packet_Ping:
		h.sendPacket(clientID, &model.Packet{
			Seq: pkt.Seq,
			Payload: &model.Packet_Pong{
				Pong: &model.Pong{Ts: p.Ping.GetTs()},
			},
		})

	case *model.Packet_SetNameReq:
		self, roomInfo, notifyIDs, code, msg := h.roomMgr.SetName(clientID, p.SetNameReq.GetName())
		h.sendPacket(clientID, &model.Packet{
			Seq: pkt.Seq,
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

	case *model.Packet_CreateRoomReq:
		roomInfo, code, msg := h.roomMgr.CreateRoom(clientID)
		h.sendPacket(clientID, &model.Packet{
			Seq: pkt.Seq,
			Payload: &model.Packet_CreateRoomResp{
				CreateRoomResp: &model.CreateRoomResp{
					Code:    code,
					Message: msg,
					Room:    roomInfo,
				},
			},
		})

	case *model.Packet_JoinRoomReq:
		roomInfo, notifyIDs, code, msg := h.roomMgr.JoinRoom(clientID, p.JoinRoomReq.GetRoomCode())
		h.sendPacket(clientID, &model.Packet{
			Seq: pkt.Seq,
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

	case *model.Packet_LeaveRoomReq:
		roomInfo, notifyIDs, code, msg := h.roomMgr.LeaveRoom(clientID)
		h.sendPacket(clientID, &model.Packet{
			Seq: pkt.Seq,
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

	default:
		log.Printf("[handler] unknown payload from %s: %T", clientID, p)
	}
}

func (h *Handler) sendPacket(clientID string, pkt *model.Packet) {
	data, err := proto.Marshal(pkt)
	if err != nil {
		log.Printf("[handler] marshal error: %v", err)
		return
	}
	h.wsMgr.Send(clientID, data)
}

func (h *Handler) pushRoomState(clientIDs []string, roomInfo *model.RoomInfo) {
	if roomInfo == nil || len(clientIDs) == 0 {
		return
	}
	data, err := proto.Marshal(&model.Packet{
		Seq: 0,
		Payload: &model.Packet_RoomStatePush{
			RoomStatePush: &model.RoomStatePush{
				Room: roomInfo,
			},
		},
	})
	if err != nil {
		log.Printf("[handler] marshal room state push error: %v", err)
		return
	}
	h.wsMgr.SendMany(clientIDs, data)
}
