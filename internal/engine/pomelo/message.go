// internal/engine/pomelo/message.go
package pomelo

// Message types (same as JS: 0=request,1=notify,2=response,3=push)
const (
	MsgRequest  = 0
	MsgNotify   = 1
	MsgResponse = 2
	MsgPush     = 3
)

// Message is a decoded pomelo message.
type Message struct {
	ID    uint32
	Type  int
	Route string
	Body  []byte
}

func msgHasID(t int) bool {
	return t == MsgRequest || t == MsgResponse
}

func msgHasRoute(t int) bool {
	return t == MsgRequest || t == MsgNotify || t == MsgPush
}

// EncodeMessage encodes a message (inner layer, goes into PackageData body).
// route is ignored for RESPONSE; id is ignored for NOTIFY/PUSH.
func EncodeMessage(id uint32, typ int, route string, body []byte) []byte {
	routeBytes := []byte(route)
	idBytes := 0
	if msgHasID(typ) {
		idBytes = varIntLen(id)
	}
	routeLen := 0
	if msgHasRoute(typ) {
		routeLen = 1 + len(routeBytes) // 1-byte length prefix
	}
	buf := make([]byte, 1+idBytes+routeLen+len(body))
	offset := 0

	buf[offset] = byte(typ << 1) // flags: (type<<1) | compressRoute(0)
	offset++

	if msgHasID(typ) {
		offset = encodeVarInt(id, buf, offset)
	}
	if msgHasRoute(typ) {
		buf[offset] = byte(len(routeBytes))
		offset++
		copy(buf[offset:], routeBytes)
		offset += len(routeBytes)
	}
	copy(buf[offset:], body)
	return buf
}

// DecodeMessage decodes a message from bytes (PackageData body).
func DecodeMessage(data []byte) Message {
	offset := 0
	flag := data[offset]
	offset++

	typ := int((flag >> 1) & 0x7)
	compressRoute := flag & 0x1

	var id uint32
	if msgHasID(typ) {
		id, offset = decodeVarInt(data, offset)
	}

	var route string
	if msgHasRoute(typ) {
		if compressRoute != 0 {
			// compressed route (2-byte code) — decode but store as empty string
			offset += 2
		} else {
			routeLen := int(data[offset])
			offset++
			route = string(data[offset : offset+routeLen])
			offset += routeLen
		}
	}

	return Message{ID: id, Type: typ, Route: route, Body: data[offset:]}
}

// MsgTypeFromStr converts "notify"/"request" string to constant.
// Defaults to MsgNotify.
func MsgTypeFromStr(s string) int {
	if s == "request" {
		return MsgRequest
	}
	return MsgNotify
}

func varIntLen(id uint32) int {
	n := 0
	for {
		n++
		id >>= 7
		if id == 0 {
			break
		}
	}
	return n
}

func encodeVarInt(id uint32, buf []byte, offset int) int {
	for {
		tmp := id & 0x7f
		id >>= 7
		if id != 0 {
			tmp |= 0x80
		}
		buf[offset] = byte(tmp)
		offset++
		if id == 0 {
			break
		}
	}
	return offset
}

func decodeVarInt(data []byte, offset int) (uint32, int) {
	var id uint32
	i := uint(0)
	for {
		m := uint32(data[offset])
		id |= (m & 0x7f) << (7 * i)
		offset++
		i++
		if m < 128 {
			break
		}
	}
	return id, offset
}
