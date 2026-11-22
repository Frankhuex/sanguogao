package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Frankhuex/sanguogao/handler"
	"github.com/Frankhuex/sanguogao/model"
	"github.com/Frankhuex/sanguogao/room"
	"github.com/Frankhuex/sanguogao/ws"
	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
)

func sendPkt(t *testing.T, ctx context.Context, conn *websocket.Conn, pkt *model.Packet) {
	t.Helper()
	data, err := proto.Marshal(pkt)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if err := conn.Write(ctx, websocket.MessageBinary, data); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func recvPkt(t *testing.T, ctx context.Context, conn *websocket.Conn) *model.Packet {
	t.Helper()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	var pkt model.Packet
	if err := proto.Unmarshal(data, &pkt); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return &pkt
}

func TestEndToEndRoomFlow(t *testing.T) {
	wsMgr := ws.NewManager()
	roomMgr := room.NewManager(room.DefaultMaxPlayers)
	h := handler.New(wsMgr, roomMgr)

	wsMgr.OnConnect = h.OnConnect
	wsMgr.OnMessage = h.OnMessage
	wsMgr.OnDisconnect = h.OnDisconnect

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", wsMgr.HandleWS)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. 客户端 A 连接并收到 WelcomePush
	connA, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial A failed: %v", err)
	}
	defer connA.CloseNow()

	welcomeA := recvPkt(t, ctx, connA).GetWelcomePush()
	if welcomeA == nil || welcomeA.GetSelf().GetId() == "" {
		t.Fatalf("expected WelcomePush for A")
	}

	// 2. A 创建房间
	sendPkt(t, ctx, connA, &model.Packet{
		Seq:     1,
		Payload: &model.Packet_CreateRoomReq{CreateRoomReq: &model.CreateRoomReq{}},
	})
	createResp := recvPkt(t, ctx, connA)
	if createResp.Seq != 1 || createResp.GetCreateRoomResp().GetCode() != model.ErrorCode_ERROR_CODE_OK {
		t.Fatalf("CreateRoom failed: %+v", createResp)
	}
	roomCode := createResp.GetCreateRoomResp().GetRoom().GetCode()

	// 3. 客户端 B 连接，用小写房间号加入
	connB, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		t.Fatalf("dial B failed: %v", err)
	}
	defer connB.CloseNow()

	welcomeB := recvPkt(t, ctx, connB).GetWelcomePush()
	if welcomeB == nil || welcomeB.GetSelf().GetId() == "" {
		t.Fatalf("expected WelcomePush for B")
	}

	sendPkt(t, ctx, connB, &model.Packet{
		Seq: 2,
		Payload: &model.Packet_JoinRoomReq{
			JoinRoomReq: &model.JoinRoomReq{RoomCode: strings.ToLower(roomCode)},
		},
	})

	// B 收到 JoinRoomResp，A 收到 RoomStatePush
	joinResp := recvPkt(t, ctx, connB)
	if joinResp.Seq != 2 || joinResp.GetJoinRoomResp().GetCode() != model.ErrorCode_ERROR_CODE_OK {
		t.Fatalf("JoinRoom failed: %+v", joinResp)
	}
	if len(joinResp.GetJoinRoomResp().GetRoom().GetPlayers()) != 2 {
		t.Fatalf("expected 2 players in JoinRoomResp")
	}

	pushA := recvPkt(t, ctx, connA).GetRoomStatePush()
	if pushA == nil || len(pushA.GetRoom().GetPlayers()) != 2 {
		t.Fatalf("expected RoomStatePush with 2 players for A, got %+v", pushA)
	}

	// 4. B 改名，A 收到 RoomStatePush 更新
	sendPkt(t, ctx, connB, &model.Packet{
		Seq: 3,
		Payload: &model.Packet_SetNameReq{
			SetNameReq: &model.SetNameReq{Name: "诸葛亮"},
		},
	})
	renameResp := recvPkt(t, ctx, connB)
	if renameResp.GetSetNameResp().GetSelf().GetName() != "诸葛亮" {
		t.Fatalf("SetNameResp mismatch: %+v", renameResp)
	}
	pushRenameA := recvPkt(t, ctx, connA).GetRoomStatePush()
	if pushRenameA == nil || pushRenameA.GetRoom().GetPlayers()[1].GetName() != "诸葛亮" {
		t.Fatalf("expected A to see B renamed to 诸葛亮, got %+v", pushRenameA)
	}

	// 5. A 掉线，B 自动收到房主移交后的 RoomStatePush
	_ = connA.Close(websocket.StatusNormalClosure, "bye")
	pushDisconnectB := recvPkt(t, ctx, connB).GetRoomStatePush()
	if pushDisconnectB == nil || len(pushDisconnectB.GetRoom().GetPlayers()) != 1 {
		t.Fatalf("expected B to receive RoomStatePush with 1 player, got %+v", pushDisconnectB)
	}
	if pushDisconnectB.GetRoom().GetHostId() != welcomeB.GetSelf().GetId() {
		t.Fatalf("expected host transferred to B, got %s", pushDisconnectB.GetRoom().GetHostId())
	}
}
