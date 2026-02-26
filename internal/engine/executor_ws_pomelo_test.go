// internal/engine/executor_ws_pomelo_test.go
package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"watchcat/internal/engine/pomelo"
)

var wsUpgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

func startPomeloMock(t *testing.T, routeResp map[string][]byte) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		pkts := pomelo.DecodePackets(raw)
		if len(pkts) == 0 || pkts[0].Type != pomelo.PackageHandshake {
			return
		}
		hsResp := []byte(`{"code":200,"sys":{"heartbeat":30}}`)
		conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageHandshake, hsResp))
		_, _, _ = conn.ReadMessage() // ACK

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			for _, pkt := range pomelo.DecodePackets(raw) {
				if pkt.Type != pomelo.PackageData {
					continue
				}
				msg := pomelo.DecodeMessage(pkt.Body)
				respBody := routeResp[msg.Route]
				if respBody == nil {
					respBody = []byte(`{"code":200}`)
				}
				var respMsg []byte
				if msg.Type == pomelo.MsgRequest {
					respMsg = pomelo.EncodeMessage(msg.ID, pomelo.MsgResponse, "", respBody)
				} else {
					respMsg = pomelo.EncodeMessage(0, pomelo.MsgPush, msg.Route, respBody)
				}
				conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageData, respMsg))
			}
		}
	}))
	t.Cleanup(ts.Close)
	return "ws" + strings.TrimPrefix(ts.URL, "http")
}

func TestExecuteWS_Pomelo_MultiRound(t *testing.T) {
	wsURL := startPomeloMock(t, map[string][]byte{
		"connector.enter": []byte(`{"uid":99,"code":200}`),
		"game.action":     []byte(`{"result":"ok","score":100}`),
	})

	cfg := map[string]any{
		"url":      wsURL,
		"protocol": "pomelo",
		"messages": []map[string]any{
			{
				"route":    "connector.enter",
				"msg_type": "notify",
				"send":     map[string]any{"token": "testtoken"},
				"expect":   map[string]any{"code": float64(200)},
				"captures": []map[string]any{
					{"source": "body", "path": "uid", "as": "uid"},
				},
			},
			{
				"route":    "game.action",
				"msg_type": "request",
				"send":     map[string]any{"uid": "{{uid}}"},
				"expect":   map[string]any{"result": "ok"},
				"assertions": []map[string]any{
					{"source": "body", "path": "result", "op": "eq", "value": "ok"},
				},
			},
		},
	}
	cfgJSON, _ := json.Marshal(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	vars := CtxMap{}
	result, captured := ExecuteWS(ctx, "test pomelo", cfgJSON, nil, nil, 5, vars)

	if result.Status != "success" {
		t.Errorf("status want success got %q: %s", result.Status, result.Error)
	}
	if captured["uid"] != "99" {
		t.Errorf("captured uid want '99' got %q", captured["uid"])
	}
}

// TestExecuteWS_Pomelo_PushBeforeResponse 模拟服务端在返回 Response 前先发送 Push 通知。
// 这正是用户遇到的场景：发送 getPlayersSum 请求后，服务端先推送了一条广播通知，
// 导致断言误判。修复后，MsgRequest 会自动跳过 Push 消息，只接受匹配的 Response。
func TestExecuteWS_Pomelo_PushBeforeResponse(t *testing.T) {
	// 自定义 mock：对 MsgRequest 先发送一条干扰 Push，再发送真正的 Response
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := wsUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		// 握手
		_, raw, err := conn.ReadMessage()
		if err != nil {
			return
		}
		pkts := pomelo.DecodePackets(raw)
		if len(pkts) == 0 || pkts[0].Type != pomelo.PackageHandshake {
			return
		}
		hsResp := []byte(`{"code":200,"sys":{"heartbeat":30}}`)
		conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageHandshake, hsResp))
		_, _, _ = conn.ReadMessage() // ACK

		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			for _, pkt := range pomelo.DecodePackets(raw) {
				if pkt.Type != pomelo.PackageData {
					continue
				}
				msg := pomelo.DecodeMessage(pkt.Body)

				if msg.Type == pomelo.MsgRequest {
					// 先发送一条干扰性的 Push 通知（模拟广播消息）
					pushBody := []byte(`{"data":[{"msg":{"info":"Congratulations!"},"type":2}]}`)
					pushMsg := pomelo.EncodeMessage(0, pomelo.MsgPush, "onBroadcast", pushBody)
					conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageData, pushMsg))

					// 再发送真正的 Response（ID 匹配）
					respBody := []byte(`{"102":50,"103":30}`)
					respMsg := pomelo.EncodeMessage(msg.ID, pomelo.MsgResponse, "", respBody)
					conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageData, respMsg))
				} else {
					// Notify → 返回 Push
					respBody := []byte(`{"uid":99,"code":200}`)
					respMsg := pomelo.EncodeMessage(0, pomelo.MsgPush, msg.Route, respBody)
					conn.WriteMessage(websocket.BinaryMessage, pomelo.EncodePacket(pomelo.PackageData, respMsg))
				}
			}
		}
	}))
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")

	cfg := map[string]any{
		"url":      wsURL,
		"protocol": "pomelo",
		"messages": []map[string]any{
			{
				"route":    "connector.entryHandler.enterPlat",
				"msg_type": "notify",
				"send":     map[string]any{"token": "test_token"},
				"expect":   map[string]any{"code": float64(200)},
				"captures": []map[string]any{
					{"source": "body", "path": "uid", "as": "uid"},
				},
			},
			{
				"route":    "area.areaHandler.getPlayersSum",
				"msg_type": "request",
				"send":     map[string]any{"gameIds": []int{102, 103}},
				// 断言：body.102 > 0（Push 通知中没有这个字段，修复前会失败）
				"assertions": []map[string]any{
					{"source": "body", "path": "102", "op": "gt", "value": "0"},
				},
			},
		},
	}
	cfgJSON, _ := json.Marshal(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, captured := ExecuteWS(ctx, "test push-before-response", cfgJSON, nil, nil, 5, CtxMap{})

	if result.Status != "success" {
		t.Errorf("status want success got %q: %s", result.Status, result.Error)
	}
	if captured["uid"] != "99" {
		t.Errorf("captured uid want '99' got %q", captured["uid"])
	}
}

func TestExecuteWS_Pomelo_AssertionFail(t *testing.T) {
	wsURL := startPomeloMock(t, map[string][]byte{
		"connector.enter": []byte(`{"code":401}`), // 非 200
	})

	cfg := map[string]any{
		"url":      wsURL,
		"protocol": "pomelo",
		"messages": []map[string]any{
			{
				"route":  "connector.enter",
				"send":   map[string]any{"token": "bad"},
				"expect": map[string]any{"code": float64(200)}, // 期望 200，服务端返回 401
			},
		},
	}
	cfgJSON, _ := json.Marshal(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, _ := ExecuteWS(ctx, "test fail", cfgJSON, nil, nil, 3, CtxMap{})

	// expect 不匹配会一直读直到超时
	if result.Status != "timeout" && result.Status != "failed" {
		t.Errorf("want timeout or failed, got %q", result.Status)
	}
}
