package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
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

// Client 封装单条 WebSocket 连接及其写协程生命周期
type Client struct {
	ID        string
	conn      *websocket.Conn
	send      chan []byte
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func newClient(parent context.Context, conn *websocket.Conn) *Client {
	ctx, cancel := context.WithCancel(parent)
	return &Client{
		ID:     generateClientID(),
		conn:   conn,
		send:   make(chan []byte, sendQueueSize),
		ctx:    ctx,
		cancel: cancel,
	}
}

func (c *Client) enqueue(data []byte) bool {
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

func generateClientID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format("150405.000000")))
	}
	return hex.EncodeToString(b)
}
