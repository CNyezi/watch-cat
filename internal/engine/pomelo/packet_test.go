// internal/engine/pomelo/packet_test.go
package pomelo

import (
	"bytes"
	"testing"
)

func TestEncodePacket(t *testing.T) {
	tests := []struct {
		name string
		typ  int
		body []byte
		want []byte
	}{
		{"heartbeat no body", PackageHeartbeat, nil, []byte{3, 0, 0, 0}},
		{"handshake ack no body", PackageHandshakeAck, nil, []byte{2, 0, 0, 0}},
		{"data with body", PackageData, []byte("hi"), []byte{4, 0, 0, 2, 'h', 'i'}},
		{"body len 3 bytes", PackageData, bytes.Repeat([]byte{0xff}, 0x010203),
			append([]byte{4, 1, 2, 3}, bytes.Repeat([]byte{0xff}, 0x010203)...)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EncodePacket(tc.typ, tc.body)
			if !bytes.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got[:min(len(got), 8)], tc.want[:min(len(tc.want), 8)])
			}
		})
	}
}

func TestDecodePackets(t *testing.T) {
	// two packets concatenated
	pkt1 := EncodePacket(PackageHandshake, []byte(`{"code":200}`))
	pkt2 := EncodePacket(PackageHeartbeat, nil)
	raw := append(pkt1, pkt2...)

	pkts := DecodePackets(raw)
	if len(pkts) != 2 {
		t.Fatalf("want 2 packets, got %d", len(pkts))
	}
	if pkts[0].Type != PackageHandshake {
		t.Errorf("pkt0 type want %d got %d", PackageHandshake, pkts[0].Type)
	}
	if string(pkts[0].Body) != `{"code":200}` {
		t.Errorf("pkt0 body want %q got %q", `{"code":200}`, string(pkts[0].Body))
	}
	if pkts[1].Type != PackageHeartbeat {
		t.Errorf("pkt1 type want %d got %d", PackageHeartbeat, pkts[1].Type)
	}
	if pkts[1].Body != nil {
		t.Errorf("pkt1 body want nil got %v", pkts[1].Body)
	}
}

func TestPacketRoundTrip(t *testing.T) {
	body := []byte(`{"sys":{"heartbeat":30}}`)
	pkts := DecodePackets(EncodePacket(PackageHandshake, body))
	if len(pkts) != 1 {
		t.Fatalf("want 1 packet")
	}
	if pkts[0].Type != PackageHandshake {
		t.Errorf("type mismatch")
	}
	if !bytes.Equal(pkts[0].Body, body) {
		t.Errorf("body mismatch")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
