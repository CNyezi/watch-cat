// internal/engine/pomelo/message_test.go
package pomelo

import (
	"bytes"
	"testing"
)

func TestEncodeDecodeNotify(t *testing.T) {
	// NOTIFY: flags=(1<<1)|0=0x02, no ID, route len+bytes, body
	body := []byte(`{"token":"abc"}`)
	route := "connector.enter"
	encoded := EncodeMessage(0, MsgNotify, route, body)

	// flags byte
	if encoded[0] != 0x02 {
		t.Errorf("flags want 0x02 got 0x%02x", encoded[0])
	}
	// route length byte
	if encoded[1] != byte(len(route)) {
		t.Errorf("route len want %d got %d", len(route), encoded[1])
	}

	msg := DecodeMessage(encoded)
	if msg.Type != MsgNotify {
		t.Errorf("type want %d got %d", MsgNotify, msg.Type)
	}
	if msg.Route != route {
		t.Errorf("route want %q got %q", route, msg.Route)
	}
	if !bytes.Equal(msg.Body, body) {
		t.Errorf("body mismatch")
	}
}

func TestEncodeDecodeRequest(t *testing.T) {
	// REQUEST: flags=(0<<1)|0=0x00, varInt ID, route, body
	body := []byte(`{"uid":1}`)
	route := "game.action"
	encoded := EncodeMessage(42, MsgRequest, route, body)

	msg := DecodeMessage(encoded)
	if msg.Type != MsgRequest {
		t.Errorf("type want %d got %d", MsgRequest, msg.Type)
	}
	if msg.ID != 42 {
		t.Errorf("id want 42 got %d", msg.ID)
	}
	if msg.Route != route {
		t.Errorf("route want %q got %q", route, msg.Route)
	}
	if !bytes.Equal(msg.Body, body) {
		t.Errorf("body mismatch")
	}
}

func TestEncodeDecodeResponse(t *testing.T) {
	// RESPONSE: flags=(2<<1)|0=0x04, varInt ID, no route, body
	body := []byte(`{"code":200}`)
	encoded := EncodeMessage(7, MsgResponse, "", body)

	msg := DecodeMessage(encoded)
	if msg.Type != MsgResponse {
		t.Errorf("type want %d got %d", MsgResponse, msg.Type)
	}
	if msg.ID != 7 {
		t.Errorf("id want 7 got %d", msg.ID)
	}
	if msg.Route != "" {
		t.Errorf("route want empty got %q", msg.Route)
	}
	if !bytes.Equal(msg.Body, body) {
		t.Errorf("body mismatch")
	}
}

func TestEncodeDecodePush(t *testing.T) {
	// PUSH: flags=(3<<1)|0=0x06, no ID, route, body
	body := []byte(`{"event":"update"}`)
	route := "game.push"
	encoded := EncodeMessage(0, MsgPush, route, body)

	msg := DecodeMessage(encoded)
	if msg.Type != MsgPush {
		t.Errorf("type want %d got %d", MsgPush, msg.Type)
	}
	if msg.Route != route {
		t.Errorf("route want %q got %q", route, msg.Route)
	}
	if !bytes.Equal(msg.Body, body) {
		t.Errorf("body mismatch")
	}
}

func TestVarIntBoundaries(t *testing.T) {
	tests := []struct {
		id      uint32
		wantLen int // expected bytes for ID encoding
	}{
		{0, 1},
		{127, 1},
		{128, 2},
		{16383, 2},
		{16384, 3},
	}
	for _, tc := range tests {
		encoded := EncodeMessage(tc.id, MsgRequest, "r", []byte("b"))
		msg := DecodeMessage(encoded)
		if msg.ID != tc.id {
			t.Errorf("id=%d: decoded %d", tc.id, msg.ID)
		}
	}
}

func TestMsgTypeFromStr(t *testing.T) {
	if MsgTypeFromStr("request") != MsgRequest {
		t.Error("request")
	}
	if MsgTypeFromStr("notify") != MsgNotify {
		t.Error("notify")
	}
	if MsgTypeFromStr("") != MsgNotify {
		t.Error("empty default notify")
	}
}
