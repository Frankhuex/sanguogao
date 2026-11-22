package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	sendQueueSize = 64
	writeTimeout  = 10 * time.Second
	pingInterval  = 30 * time.Second
	pingTimeout   = 5 * time.Second
)

type Client struct {
	ID        string
	conn      *websocket.Conn
	send      chan []byte
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func (c *Client) close(status websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		c.cancel()
		_ = c.conn.Close(status, reason)
	})
}

func (c *Client) writeLoop() {
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case data := <-c.send:
			writeCtx, cancel := context.WithTimeout(c.ctx, writeTimeout)
			err := c.conn.Write(writeCtx, websocket.MessageBinary, data)
			cancel()
			if err != nil {
				c.close(websocket.StatusGoingAway, "write failed")
				return
			}
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(c.ctx, pingTimeout)
			err := c.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				c.close(websocket.StatusGoingAway, "ping timeout")
				return
			}
		}
	}
}

type Manager struct {
	mu      sync.RWMutex
	clients map[string]*Client

	OnConnect    func(clientID string)
	OnMessage    func(clientID string, data []byte)
	OnDisconnect func(clientID string)
}

func NewManager() *Manager {
	return &Manager{
		clients: make(map[string]*Client),
	}
}

func (m *Manager) add(c *Client) {
	m.mu.Lock()
	m.clients[c.ID] = c
	total := len(m.clients)
	m.mu.Unlock()

	log.Printf("[ws] connected: id=%s (online=%d)", c.ID, total)
}

func (m *Manager) remove(c *Client) {
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

// Send 向单个客户端发送二进制消息（非阻塞，队列满时断开慢客户端）
func (m *Manager) Send(clientID string, data []byte) bool {
	m.mu.RLock()
	c, ok := m.clients[clientID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	return enqueue(c, data)
}

// SendMany 向多个客户端批量发送同一份二进制消息
func (m *Manager) SendMany(clientIDs []string, data []byte) {
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
		enqueue(c, data)
	}
}

func enqueue(c *Client, data []byte) bool {
	select {
	case <-c.ctx.Done():
		return false
	case c.send <- data:
		return true
	default:
		log.Printf("[ws] send queue full, closing slow client: id=%s", c.ID)
		c.close(websocket.StatusPolicyViolation, "send buffer full")
		return false
	}
}

// HandleWS 处理 WebSocket 升级、消息读写与断线清理
func (m *Manager) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // 开发阶段允许跨域连接
	})
	if err != nil {
		log.Printf("[ws] accept error: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())
	client := &Client{
		ID:     generateClientID(),
		conn:   conn,
		send:   make(chan []byte, sendQueueSize),
		ctx:    ctx,
		cancel: cancel,
	}

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

func generateClientID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 极端情况下退化为时间戳
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}
