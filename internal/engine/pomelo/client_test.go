// internal/engine/pomelo/client_test.go
package pomelo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// mockServer 是一个简单的 pomelo 服务端，用于测试。
type mockServer struct {
	ts            *httptest.Server
	heartbeatRecv atomic.Int32  // server 收到的 heartbeat 数
	routeResp     map[string][]byte // route → 返回的 body JSON
}

func newMockServer(t *testing.T, routeResp map[string][]byte) *mockServer {
	t.Helper()
	m := &mockServer{routeResp: routeResp}
	m.ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 1. 收握手
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		pkts := DecodePackets(raw)
		if len(pkts) == 0 || pkts[0].Type != PackageHandshake {
			return
		}
		// 2. 回握手响应（heartbeat=1s）
		hsResp := []byte(`{"code":200,"sys":{"heartbeat":1}}`)
		conn.WriteMessage(websocket.BinaryMessage, EncodePacket(PackageHandshake, hsResp))

		// 3. 收握手 ACK
		_, _, _ = conn.ReadMessage()

		// 4. 主循环
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			for _, pkt := range DecodePackets(raw) {
				switch pkt.Type {
				case PackageHeartbeat:
					m.heartbeatRecv.Add(1)
					conn.WriteMessage(websocket.BinaryMessage, EncodePacket(PackageHeartbeat, nil))
				case PackageData:
					msg := DecodeMessage(pkt.Body)
					respBody := m.routeResp[msg.Route]
					if respBody == nil {
						respBody = []byte(`{"code":200}`)
					}
					var respMsg []byte
					if msg.Type == MsgRequest {
						// RESPONSE 带同一 ID
						respMsg = EncodeMessage(msg.ID, MsgResponse, "", respBody)
					} else {
						// PUSH 带同一 route
						respMsg = EncodeMessage(0, MsgPush, msg.Route, respBody)
					}
					conn.WriteMessage(websocket.BinaryMessage, EncodePacket(PackageData, respMsg))
				}
			}
		}
	}))
	return m
}

func (m *mockServer) wsURL() string {
	return "ws" + strings.TrimPrefix(m.ts.URL, "http")
}

func (m *mockServer) close() {
	m.ts.Close()
}

// ──────────────────────────────────────────────────────────────
// Tests
// ──────────────────────────────────────────────────────────────

func TestDial(t *testing.T) {
	srv := newMockServer(t, nil)
	defer srv.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := Dial(ctx, srv.wsURL(), nil, nil)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	client.Close()
}

func TestSendNotify(t *testing.T) {
	srv := newMockServer(t, map[string][]byte{
		"connector.enter": []byte(`{"uid":42,"code":200}`),
	})
	defer srv.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := Dial(ctx, srv.wsURL(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	_, err = client.Send("connector.enter", MsgNotify, []byte(`{"token":"test"}`))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg, err := client.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if msg.Route != "connector.enter" {
		t.Errorf("route want 'connector.enter' got %q", msg.Route)
	}
	if string(msg.Body) != `{"uid":42,"code":200}` {
		t.Errorf("body want '{\"uid\":42,\"code\":200}' got %q", string(msg.Body))
	}
}

func TestSendRequest(t *testing.T) {
	srv := newMockServer(t, map[string][]byte{
		"game.action": []byte(`{"result":"ok"}`),
	})
	defer srv.close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client, err := Dial(ctx, srv.wsURL(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	_, err = client.Send("game.action", MsgRequest, []byte(`{"uid":1}`))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	msg, err := client.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if msg.Type != MsgResponse {
		t.Errorf("type want MsgResponse got %d", msg.Type)
	}
	if string(msg.Body) != `{"result":"ok"}` {
		t.Errorf("body want '{\"result\":\"ok\"}' got %q", string(msg.Body))
	}
}

func TestHeartbeat(t *testing.T) {
	srv := newMockServer(t, nil)
	defer srv.close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := Dial(ctx, srv.wsURL(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	// 等待 2.5s（心跳间隔 1s，期望至少 2 次）
	time.Sleep(2500 * time.Millisecond)

	got := srv.heartbeatRecv.Load()
	if got < 2 {
		t.Errorf("want at least 2 heartbeats, got %d", got)
	}
}

func TestCtxCancel(t *testing.T) {
	srv := newMockServer(t, nil)
	defer srv.close()

	ctx, cancel := context.WithCancel(context.Background())

	client, err := Dial(ctx, srv.wsURL(), nil, nil)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// 取消后 Close
	cancel()
	client.Close()

	// Recv 应该返回 error
	_, err = client.Recv()
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}
