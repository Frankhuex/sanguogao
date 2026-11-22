package room

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Frankhuex/sanguogao/model"
)

func TestRoomLifecycle(t *testing.T) {
	mgr := NewManager(DefaultMaxPlayers)

	// 1. AddPlayer 分配默认昵称
	p1 := mgr.AddPlayer("aaaa1234")
	if p1.Name != "游客_1234" {
		t.Fatalf("expected default name 游客_1234, got %s", p1.Name)
	}
	p2 := mgr.AddPlayer("bbbb5678")

	// 2. 创建房间（未改名也允许）
	roomInfo, code, _ := mgr.CreateRoom(p1.Id)
	if code != model.ErrorCode_ERROR_CODE_OK {
		t.Fatalf("CreateRoom failed: %v", code)
	}
	if len(roomInfo.Code) != RoomCodeLength {
		t.Fatalf("expected 6-char room code, got %q", roomInfo.Code)
	}
	if roomInfo.HostId != p1.Id || len(roomInfo.Players) != 1 {
		t.Fatalf("unexpected room state: %+v", roomInfo)
	}

	// 3. 大小写不敏感加入房间
	lowerCode := strings.ToLower(roomInfo.Code)
	joinedRoom, notifyIDs, code, _ := mgr.JoinRoom(p2.Id, " "+lowerCode+" ")
	if code != model.ErrorCode_ERROR_CODE_OK {
		t.Fatalf("JoinRoom failed: %v", code)
	}
	if len(joinedRoom.Players) != 2 {
		t.Fatalf("expected 2 players, got %d", len(joinedRoom.Players))
	}
	if len(notifyIDs) != 1 || notifyIDs[0] != p1.Id {
		t.Fatalf("expected notify [%s], got %v", p1.Id, notifyIDs)
	}

	// 4. 房内改名且允许同名，推送给房间内其他人
	self1, roomAfterRename, notifyIDs, code, _ := mgr.SetName(p1.Id, "曹操")
	if code != model.ErrorCode_ERROR_CODE_OK || self1.Name != "曹操" {
		t.Fatalf("SetName p1 failed: code=%v self=%+v", code, self1)
	}
	if roomAfterRename == nil || len(notifyIDs) != 1 || notifyIDs[0] != p2.Id {
		t.Fatalf("expected notify [%s] on rename, got %v", p2.Id, notifyIDs)
	}

	self2, _, _, code, _ := mgr.SetName(p2.Id, "曹操")
	if code != model.ErrorCode_ERROR_CODE_OK || self2.Name != "曹操" {
		t.Fatalf("expected duplicate name allowed, got code=%v", code)
	}

	// 5. 房主掉线 -> 自动退房 + 房主顺延给 p2 + 通知 p2
	discRes := mgr.RemovePlayer(p1.Id)
	if discRes.RoomInfo == nil || discRes.RoomInfo.HostId != p2.Id || len(discRes.RoomInfo.Players) != 1 {
		t.Fatalf("expected host transfer to p2, got %+v", discRes.RoomInfo)
	}
	if len(discRes.NotifyIDs) != 1 || discRes.NotifyIDs[0] != p2.Id {
		t.Fatalf("expected notify [%s], got %v", p2.Id, discRes.NotifyIDs)
	}

	// 6. 最后一人离开 -> 房间销毁
	afterLast, notifyIDs, code, _ := mgr.LeaveRoom(p2.Id)
	if code != model.ErrorCode_ERROR_CODE_OK || afterLast != nil || len(notifyIDs) != 0 {
		t.Fatalf("expected room destroyed, got room=%+v notify=%v code=%v", afterLast, notifyIDs, code)
	}

	// 确认房间已不存在
	p3 := mgr.AddPlayer("cccc9999")
	_, _, code, _ = mgr.JoinRoom(p3.Id, roomInfo.Code)
	if code != model.ErrorCode_ERROR_CODE_ROOM_NOT_FOUND {
		t.Fatalf("expected ROOM_NOT_FOUND, got %v", code)
	}
}

func TestRoomMaxPlayers(t *testing.T) {
	mgr := NewManager(10)
	host := mgr.AddPlayer("player00")
	r, code, _ := mgr.CreateRoom(host.Id)
	if code != model.ErrorCode_ERROR_CODE_OK {
		t.Fatalf("CreateRoom failed: %v", code)
	}

	for i := 1; i < 10; i++ {
		p := mgr.AddPlayer(fmt.Sprintf("player%02d", i))
		_, _, code, _ := mgr.JoinRoom(p.Id, r.Code)
		if code != model.ErrorCode_ERROR_CODE_OK {
			t.Fatalf("player %d JoinRoom failed: %v", i, code)
		}
	}

	extra := mgr.AddPlayer("player10")
	_, _, code, _ = mgr.JoinRoom(extra.Id, r.Code)
	if code != model.ErrorCode_ERROR_CODE_ROOM_FULL {
		t.Fatalf("expected ROOM_FULL for 11th player, got %v", code)
	}
}
