package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tidwall/gjson"
	"watchcat/internal/engine/pomelo"
)

const maxTraceMessages = 100

type wsTraceMsg struct {
	Time string `json:"time"`
	Data string `json:"data"`
}

// ExecuteWS executes a WebSocket step.
// Supports two modes:
//   - Multi-round (messages array): each element is an independent send→receive round
//     over the same connection; captures from earlier rounds are available in later rounds.
//   - Single-round (backward compat): uses top-level send/expect/captures/assertions fields.
func ExecuteWS(ctx context.Context, name string, stepCfg, capturesJSON, assertionsJSON json.RawMessage, timeout int, vars CtxMap) (*StepResult, CtxMap) {
	start := time.Now()
	result := &StepResult{Name: name, Type: "ws", Status: "success"}
	captured := make(CtxMap)

	var cfg WSConfig
	if err := json.Unmarshal(stepCfg, &cfg); err != nil {
		result.Status = "failed"
		result.Error = fmt.Sprintf("invalid ws config: %v", err)
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}

	// Render template variables in URL and headers
	cfg.URL = Render(cfg.URL, vars)
	for k, v := range cfg.Headers {
		cfg.Headers[k] = Render(v, vars)
	}

	// Apply step-level timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// Build request headers
	reqHeader := http.Header{}
	for k, v := range cfg.Headers {
		reqHeader.Set(k, v)
	}

	// ── Pomelo protocol branch ────────────────────────────────────────────
	if cfg.Protocol == "pomelo" {
		return executePomeloWS(ctx, start, name, result, captured, cfg, vars, reqHeader)
	}

	// Connect
	dialer := websocket.Dialer{}
	conn, _, err := dialer.DialContext(ctx, cfg.URL, reqHeader)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "timeout"
			result.Error = "ws connect timeout"
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("ws connect failed: %v", err)
		}
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}
	defer conn.Close()

	if deadline, ok := ctx.Deadline(); ok {
		conn.SetReadDeadline(deadline)
	}

	var lastRecv string

	if len(cfg.Messages) > 0 {
		// ── Multi-round mode ──────────────────────────────────────────────────
		type roundDetail struct {
			Round int    `json:"round"`
			Send  string `json:"send"`
			Recv  string `json:"recv,omitempty"`
		}
		roundDetails := make([]roundDetail, 0, len(cfg.Messages))
		completedRounds := 0
		phase := "connecting"

		wsReqMap := map[string]any{
			"url":              cfg.URL,
			"rounds":           len(cfg.Messages),
			"completed_rounds": completedRounds,
			"phase":            phase,
			"messages":         roundDetails,
		}
		result.Request = wsReqMap

		updateProgress := func() {
			wsReqMap["completed_rounds"] = completedRounds
			wsReqMap["phase"] = phase
			wsReqMap["messages"] = roundDetails
		}

		// roundVars starts as a copy of vars; captures accumulate here across rounds
		roundVars := Merge(CtxMap{}, vars)

		for i, msg := range cfg.Messages {
			sendStr := Render(string(msg.Send), roundVars)
			rd := roundDetail{Round: i + 1, Send: sendStr}

			if len(msg.Send) > 0 && sendStr != "" && sendStr != "null" {
				phase = fmt.Sprintf("round %d: sending", i+1)
				updateProgress()
				if err := conn.WriteMessage(websocket.TextMessage, []byte(sendStr)); err != nil {
					result.Status = "failed"
					result.Error = fmt.Sprintf("round %d: ws send failed: %v", i+1, err)
					roundDetails = append(roundDetails, rd)
					updateProgress()
					result.DurationMs = time.Since(start).Milliseconds()
					return result, captured
				}
			}

			phase = fmt.Sprintf("round %d: receiving", i+1)
			updateProgress()

			// Read until a message matching expect is received
			var traceMessages []wsTraceMsg
			for {
				_, data, err := conn.ReadMessage()
				if err != nil {
					if ctx.Err() == context.DeadlineExceeded {
						result.Status = "timeout"
						result.Error = fmt.Sprintf("round %d: ws recv timeout", i+1)
					} else {
						result.Status = "failed"
						result.Error = fmt.Sprintf("round %d: ws recv failed: %v", i+1, err)
					}
					roundDetails = append(roundDetails, rd)
					updateProgress()
					if len(traceMessages) > 0 {
						result.Response = map[string]any{"messages": traceMessages}
					}
					result.DurationMs = time.Since(start).Milliseconds()
					return result, captured
				}
				lastRecv = string(data)
				if len(traceMessages) < maxTraceMessages {
					traceMessages = append(traceMessages, wsTraceMsg{
						Time: time.Now().Format(time.RFC3339),
						Data: truncate(lastRecv, 4096),
					})
				}
				if len(msg.Expect) == 0 || matchExpect(lastRecv, msg.Expect) {
					break
				}
			}
			rd.Recv = truncate(lastRecv, 4096)
			roundDetails = append(roundDetails, rd)

			// Captures: write into roundVars so subsequent rounds can use them
			if len(msg.Captures) > 0 {
				capturesRaw, _ := json.Marshal(msg.Captures)
				roundCaptured := executeCaptures(capturesRaw, lastRecv, nil, roundVars)
				for k, v := range roundCaptured {
					captured[k] = v // also collect for step-level output
				}
			}

			// Assertions on this round's response
			if len(msg.Assertions) > 0 {
				phase = fmt.Sprintf("round %d: asserting", i+1)
				updateProgress()
				assertsRaw, _ := json.Marshal(msg.Assertions)
				if err := executeAssertions(assertsRaw, 0, lastRecv, nil, roundVars); err != nil {
					result.Status = "failed"
					result.Error = fmt.Sprintf("round %d: %v", i+1, err)
					updateProgress()
					result.DurationMs = time.Since(start).Milliseconds()
					return result, captured
				}
			}

			completedRounds++
		}

		phase = "completed"
		updateProgress()
		result.Response = map[string]any{"body": truncate(lastRecv, 4096)}
		result.Captures = captured

	} else {
		// ── Single-round backward compat ──────────────────────────────────────
		sendStr := Render(string(cfg.Send), vars)
		result.Request = map[string]any{
			"url":    cfg.URL,
			"send":   sendStr,
			"expect": cfg.Expect,
		}

		if len(cfg.Send) > 0 && sendStr != "" && sendStr != "null" {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(sendStr)); err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("ws send failed: %v", err)
				result.DurationMs = time.Since(start).Milliseconds()
				return result, captured
			}
		}

		var traceMessages []wsTraceMsg
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					result.Status = "timeout"
					result.Error = "ws recv timeout"
				} else {
					result.Status = "failed"
					result.Error = fmt.Sprintf("ws recv failed: %v", err)
				}
				if len(traceMessages) > 0 {
					result.Response = map[string]any{"messages": traceMessages}
				}
				result.DurationMs = time.Since(start).Milliseconds()
				return result, captured
			}
			lastRecv = string(data)
			if len(traceMessages) < maxTraceMessages {
				traceMessages = append(traceMessages, wsTraceMsg{
					Time: time.Now().Format(time.RFC3339),
					Data: truncate(lastRecv, 4096),
				})
			}
			if len(cfg.Expect) == 0 || matchExpect(lastRecv, cfg.Expect) {
				break
			}
		}

		result.Response = map[string]any{"body": truncate(lastRecv, 4096)}
		result.Captures = executeCaptures(capturesJSON, lastRecv, nil, captured)
		if err := executeAssertions(assertionsJSON, 0, lastRecv, nil, vars); err != nil {
			result.Status = "failed"
			result.Error = err.Error()
			result.DurationMs = time.Since(start).Milliseconds()
			return result, captured
		}
	}

	result.DurationMs = time.Since(start).Milliseconds()
	return result, captured
}

// executePomeloWS handles WebSocket steps using the pomelo binary protocol.
func executePomeloWS(ctx context.Context, start time.Time, name string, result *StepResult, captured CtxMap, cfg WSConfig, vars CtxMap, headers http.Header) (*StepResult, CtxMap) {
	type roundDetail struct {
		Round int    `json:"round"`
		Route string `json:"route"`
		Send  string `json:"send"`
		Recv  string `json:"recv,omitempty"`
	}
	roundDetails := make([]roundDetail, 0, len(cfg.Messages))
	completedRounds := 0
	phase := "connecting"

	reqMap := map[string]any{
		"url":              cfg.URL,
		"protocol":         "pomelo",
		"rounds":           len(cfg.Messages),
		"completed_rounds": completedRounds,
		"phase":            phase,
		"messages":         roundDetails,
	}
	result.Request = reqMap

	updateProgress := func() {
		reqMap["completed_rounds"] = completedRounds
		reqMap["phase"] = phase
		reqMap["messages"] = roundDetails
	}

	client, err := pomelo.Dial(ctx, cfg.URL, headers, cfg.HandshakeData)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			result.Status = "timeout"
			result.Error = "pomelo connect timeout"
		} else {
			result.Status = "failed"
			result.Error = fmt.Sprintf("pomelo connect: %v", err)
		}
		updateProgress()
		result.DurationMs = time.Since(start).Milliseconds()
		return result, captured
	}
	defer client.Close()

	// When ctx is cancelled/timed out, close the connection to unblock Recv.
	go func() {
		<-ctx.Done()
		client.Close()
	}()

	roundVars := Merge(CtxMap{}, vars)
	var lastRecv string

	for i, msg := range cfg.Messages {
		sendStr := Render(string(msg.Send), roundVars)
		hasSend := len(msg.Send) > 0 && sendStr != "" && sendStr != "null"
		hasRecv := hasSend || len(msg.Expect) > 0 || len(msg.Captures) > 0 || len(msg.Assertions) > 0
		if !hasRecv {
			continue // 空轮次，跳过
		}

		rd := roundDetail{Round: i + 1, Route: msg.Route, Send: sendStr}

		var reqID uint32
		isRequest := pomelo.MsgTypeFromStr(msg.MsgType) == pomelo.MsgRequest

		if hasSend {
			phase = fmt.Sprintf("round %d: sending", i+1)
			updateProgress()
			msgType := pomelo.MsgTypeFromStr(msg.MsgType)
			reqID, err = client.Send(msg.Route, msgType, []byte(sendStr))
			if err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("round %d: pomelo send: %v", i+1, err)
				roundDetails = append(roundDetails, rd)
				updateProgress()
				result.DurationMs = time.Since(start).Milliseconds()
				return result, captured
			}
		}

		phase = fmt.Sprintf("round %d: receiving", i+1)
		updateProgress()

		// Read until a matching message is received.
		// When we sent a MsgRequest, skip Push/Notify messages and only accept
		// the MsgResponse with matching ID. This prevents server-pushed notifications
		// (e.g. broadcast announcements) from being mistaken for the response.
		var traceMessages []wsTraceMsg
		for {
			pMsg, err := client.Recv()
			if err != nil {
				if ctx.Err() == context.DeadlineExceeded {
					result.Status = "timeout"
					result.Error = fmt.Sprintf("round %d: pomelo recv timeout", i+1)
				} else {
					result.Status = "failed"
					result.Error = fmt.Sprintf("round %d: pomelo recv: %v", i+1, err)
				}
				roundDetails = append(roundDetails, rd)
				updateProgress()
				if len(traceMessages) > 0 {
					result.Response = map[string]any{"messages": traceMessages}
				}
				result.DurationMs = time.Since(start).Milliseconds()
				return result, captured
			}

			// 收集所有收到的消息（含被跳过的推送通知）
			if len(traceMessages) < maxTraceMessages {
				traceMessages = append(traceMessages, wsTraceMsg{
					Time: time.Now().Format(time.RFC3339),
					Data: truncate(string(pMsg.Body), 4096),
				})
			}

			// For MsgRequest: filter out Push/Notify, only accept matching Response
			if isRequest && hasSend {
				if pMsg.Type == pomelo.MsgPush || pMsg.Type == pomelo.MsgNotify {
					continue // 跳过服务端推送通知
				}
				if pMsg.Type == pomelo.MsgResponse && pMsg.ID != reqID {
					continue // 跳过不匹配的响应（理论上不应发生）
				}
			}

			lastRecv = string(pMsg.Body)
			if len(msg.Expect) == 0 || matchExpect(lastRecv, msg.Expect) {
				break
			}
		}
		rd.Recv = truncate(lastRecv, 4096)
		roundDetails = append(roundDetails, rd)

		// Captures
		if len(msg.Captures) > 0 {
			capturesRaw, _ := json.Marshal(msg.Captures)
			roundCaptured := executeCaptures(capturesRaw, lastRecv, nil, roundVars)
			for k, v := range roundCaptured {
				captured[k] = v
			}
		}

		// Assertions
		if len(msg.Assertions) > 0 {
			phase = fmt.Sprintf("round %d: asserting", i+1)
			updateProgress()
			assertsRaw, _ := json.Marshal(msg.Assertions)
			if err := executeAssertions(assertsRaw, 0, lastRecv, nil, roundVars); err != nil {
				result.Status = "failed"
				result.Error = fmt.Sprintf("round %d: %v", i+1, err)
				updateProgress()
				result.DurationMs = time.Since(start).Milliseconds()
				return result, captured
			}
		}

		completedRounds++
	}

	phase = "completed"
	updateProgress()
	result.Response = map[string]any{"body": truncate(lastRecv, 4096)}
	result.Captures = captured
	result.DurationMs = time.Since(start).Milliseconds()
	return result, captured
}

// matchExpect checks if all expected fields match in the given JSON message.
func matchExpect(msg string, expect map[string]any) bool {
	for path, expectedVal := range expect {
		r := gjson.Get(msg, path)
		if !r.Exists() {
			return false
		}
		if fmt.Sprintf("%v", r.Value()) != fmt.Sprintf("%v", expectedVal) {
			return false
		}
	}
	return true
}
