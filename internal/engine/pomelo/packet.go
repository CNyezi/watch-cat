// internal/engine/pomelo/packet.go
package pomelo

// Packet types
const (
	PackageHandshake    = 1
	PackageHandshakeAck = 2
	PackageHeartbeat    = 3
	PackageData         = 4
	PackageKick         = 5
)

// Packet is a decoded pomelo packet.
type Packet struct {
	Type int
	Body []byte // nil for empty body
}

// EncodePacket encodes a packet.
// Format: [type:1B][length:3B big-endian][body:NB]
func EncodePacket(typ int, body []byte) []byte {
	length := len(body)
	buf := make([]byte, 4+length)
	buf[0] = byte(typ)
	buf[1] = byte((length >> 16) & 0xff)
	buf[2] = byte((length >> 8) & 0xff)
	buf[3] = byte(length & 0xff)
	copy(buf[4:], body)
	return buf
}

// DecodePackets decodes one or more packets from raw bytes.
func DecodePackets(data []byte) []Packet {
	var packets []Packet
	offset := 0
	for offset+4 <= len(data) {
		typ := int(data[offset])
		length := int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		offset += 4
		if offset+length > len(data) {
			break
		}
		var body []byte
		if length > 0 {
			body = make([]byte, length)
			copy(body, data[offset:offset+length])
		}
		offset += length
		packets = append(packets, Packet{Type: typ, Body: body})
	}
	return packets
}
