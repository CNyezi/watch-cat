// internal/engine/pomelo/client.go
package pomelo

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Client is a connected pomelo client.
type Client struct {
	conn      *websocket.Conn
	nextID    atomic.Uint32
	writeMu   sync.Mutex
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// Dial connects to a pomelo server, performs handshake, and starts the heartbeat goroutine.
// handshakeData is merged into the {"sys":{...}} handshake body (may be nil).
func Dial(ctx context.Context, url string, headers http.Header, handshakeData map[string]any) (*Client, error) {
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, fmt.Errorf("pomelo dial: %w", err)
	}

	c := &Client{conn: conn}
	interval, err := c.handshake(handshakeData)
	if err != nil {
		conn.Close()
		return nil, err
	}

	hbCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.heartbeatLoop(hbCtx, interval)
	return c, nil
}

func (c *Client) handshake(extra map[string]any) (time.Duration, error) {
	data := map[string]any{
		"sys": map[string]any{"type": "js-websocket", "version": "0.0.1"},
	}
	for k, v := range extra {
		data[k] = v
	}
	body, _ := json.Marshal(data)
	if err := c.writeRaw(EncodePacket(PackageHandshake, body)); err != nil {
		return 0, fmt.Errorf("handshake send: %w", err)
	}

	_, raw, err := c.conn.ReadMessage()
	if err != nil {
		return 0, fmt.Errorf("handshake recv: %w", err)
	}
	pkts := DecodePackets(raw)
	if len(pkts) == 0 || pkts[0].Type != PackageHandshake {
		return 0, fmt.Errorf("unexpected handshake packet type")
	}

	var resp struct {
		Code int `json:"code"`
		Sys  struct {
			Heartbeat int `json:"heartbeat"`
		} `json:"sys"`
	}
	if err := json.Unmarshal(pkts[0].Body, &resp); err != nil {
		return 0, fmt.Errorf("handshake parse: %w", err)
	}
	if resp.Code != 200 {
		return 0, fmt.Errorf("handshake rejected: code=%d", resp.Code)
	}

	if err := c.writeRaw(EncodePacket(PackageHandshakeAck, nil)); err != nil {
		return 0, fmt.Errorf("handshake ack: %w", err)
	}

	interval := time.Duration(resp.Sys.Heartbeat) * time.Second
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return interval, nil
}

func (c *Client) heartbeatLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			_ = c.writeRaw(EncodePacket(PackageHeartbeat, nil))
		case <-ctx.Done():
			return
		}
	}
}

// Send encodes and sends a pomelo message.
// msgType: MsgNotify or MsgRequest.
// Returns the message ID assigned to this message (relevant for MsgRequest matching).
func (c *Client) Send(route string, msgType int, body []byte) (uint32, error) {
	id := c.nextID.Add(1)
	msgData := EncodeMessage(id, msgType, route, body)
	return id, c.writeRaw(EncodePacket(PackageData, msgData))
}

// Recv reads the next DATA message, skipping heartbeats.
// Returns the full decoded Message so callers can inspect Type and ID.
func (c *Client) Recv() (*Message, error) {
	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		for _, pkt := range DecodePackets(raw) {
			switch pkt.Type {
			case PackageHeartbeat:
				// echo back
				_ = c.writeRaw(EncodePacket(PackageHeartbeat, nil))
			case PackageKick:
				return nil, fmt.Errorf("kicked by server")
			case PackageData:
				msg := DecodeMessage(pkt.Body)
				return &msg, nil
			}
		}
	}
}

// Close shuts down the client.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		c.conn.Close()
	})
}

func (c *Client) writeRaw(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}
