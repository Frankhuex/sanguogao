package main

import (
	"log"
	"net/http"

	"github.com/Frankhuex/sanguogao/internal/handler"
	"github.com/Frankhuex/sanguogao/internal/room"
	"github.com/Frankhuex/sanguogao/internal/ws"
)

func main() {
	wsMgr := ws.NewManager()
	roomMgr := room.NewManager(room.DefaultMaxPlayers)
	h := handler.New(wsMgr, roomMgr)

	wsMgr.OnConnect = h.OnConnect
	wsMgr.OnMessage = h.OnMessage
	wsMgr.OnDisconnect = h.OnDisconnect

	http.HandleFunc("/ws", wsMgr.HandleWS)
	http.Handle("/proto/", http.StripPrefix("/proto/", http.FileServer(http.Dir("proto"))))
	http.Handle("/", http.FileServer(http.Dir("web")))

	addr := ":8080"
	log.Printf("server listening on http://localhost%s (ws: ws://localhost%s/ws)", addr, addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
