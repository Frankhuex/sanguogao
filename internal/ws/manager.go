package ws

import (
	"log"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// WSManager 管理所有在线 WebSocket 连接
type WSManager struct {
	mu      sync.RWMutex
	clients map[string]*Client

	OnConnect    func(clientID string)
	OnMessage    func(clientID string, data []byte)
	OnDisconnect func(clientID string)
}

func NewManager() *WSManager {
	return &WSManager{
		clients: make(map[string]*Client),
	}
}

// Send 向单个客户端发送二进制消息
func (m *WSManager) Send(clientID string, data []byte) bool {
	m.mu.RLock()
	c, ok := m.clients[clientID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return c.enqueue(data)
}

// SendMany 向多个客户端批量发送同一份二进制消息
func (m *WSManager) SendMany(clientIDs []string, data []byte) {
	if len(clientIDs) == 0 {
		return
	}
	targets := make([]*Client, 0, len(clientIDs))
	m.mu.RLock()
	for _, id := range clientIDs {
		if c, ok := m.clients[id]; ok {
			targets = append(targets, c)
		}
	}
	m.mu.RUnlock()

	for _, c := range targets {
		c.enqueue(data)
	}
}

// HandleWS 处理 WebSocket 升级、消息读取与断线清理
func (m *WSManager) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 开发阶段允许跨域连接
	})
	if err != nil {
		log.Printf("[ws] accept error: %v", err)
		return
	}

	client := newClient(r.Context(), conn)
	m.add(client)
	defer m.remove(client)

	go client.writeLoop()

	if m.OnConnect != nil {
		m.OnConnect(client.ID)
	}

	for {
		msgType, data, err := conn.Read(client.ctx)
		if err != nil {
			return
		}
		if msgType != websocket.MessageBinary {
			continue
		}
		if m.OnMessage != nil {
			m.OnMessage(client.ID, data)
		}
	}
}

func (m *WSManager) add(c *Client) {
	m.mu.Lock()
	m.clients[c.ID] = c
	total := len(m.clients)
	m.mu.Unlock()

	log.Printf("[ws] connected: id=%s (online=%d)", c.ID, total)
}

func (m *WSManager) remove(c *Client) {
	m.mu.Lock()
	removed := false
	if cur, ok := m.clients[c.ID]; ok && cur == c {
		delete(m.clients, c.ID)
		removed = true
	}
	total := len(m.clients)
	m.mu.Unlock()

	c.close(websocket.StatusNormalClosure, "disconnected")
	log.Printf("[ws] disconnected: id=%s (online=%d)", c.ID, total)

	if removed && m.OnDisconnect != nil {
		m.OnDisconnect(c.ID)
	}
}
