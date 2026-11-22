package handler

import (
	"log"

	"github.com/Frankhuex/sanguogao/internal/room"
	"github.com/Frankhuex/sanguogao/internal/ws"
	"github.com/Frankhuex/sanguogao/model"
	"google.golang.org/protobuf/proto"
)

type Handler struct {
	wsMgr   *ws.WSManager
	roomMgr *room.Manager
}

func New(wsMgr *ws.WSManager, roomMgr *room.Manager) *Handler {
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
	case *model.Packet_SetNameReq:
		h.handleSetName(clientID, pkt.Seq, p.SetNameReq)
	case *model.Packet_CreateRoomReq:
		h.handleCreateRoom(clientID, pkt.Seq)
	case *model.Packet_JoinRoomReq:
		h.handleJoinRoom(clientID, pkt.Seq, p.JoinRoomReq)
	case *model.Packet_LeaveRoomReq:
		h.handleLeaveRoom(clientID, pkt.Seq)

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

func (h *Handler) broadcastPacket(clientIDs []string, pkt *model.Packet) {
	if len(clientIDs) == 0 {
		return
	}
	data, err := proto.Marshal(pkt)
	if err != nil {
		log.Printf("[handler] marshal broadcast error: %v", err)
		return
	}
	h.wsMgr.SendMany(clientIDs, data)
}
