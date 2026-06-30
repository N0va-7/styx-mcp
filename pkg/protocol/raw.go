package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"

	"mcp-stowaway/pkg/crypto"
)

// RawProto is the plain TCP transport negotiator.
type RawProto struct{}

// CNegotiate performs client-side negotiation (no-op for raw TCP).
func (proto *RawProto) CNegotiate() error { return nil }

// SNegotiate performs server-side negotiation (no-op for raw TCP).
func (proto *RawProto) SNegotiate() error { return nil }

// RawMessage implements Message over raw TCP.
type RawMessage struct {
	UUID         string
	Conn         net.Conn
	CryptoSecret []byte
	HeaderBuffer []byte
	DataBuffer   []byte
}

// ConstructHeader is a no-op for raw TCP; the full header is built in ConstructData.
func (message *RawMessage) ConstructHeader() {}

// ConstructData serializes the payload and prepares the full header buffer.
func (message *RawMessage) ConstructData(header *Header, mess interface{}, isPass bool) error {
	var headerBuf, dataBuf bytes.Buffer

	// Build payload.
	if !isPass {
		if mess == nil {
			return errors.New("nil message payload")
		}
		val := reflect.ValueOf(mess)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		typ := val.Type()

		for i := 0; i < typ.NumField(); i++ {
			field := val.Field(i)
			inter := field.Interface()

			switch value := inter.(type) {
			case string:
				dataBuf.WriteString(value)
			case uint16:
				b := make([]byte, 2)
				binary.BigEndian.PutUint16(b, value)
				dataBuf.Write(b)
			case uint32:
				b := make([]byte, 4)
				binary.BigEndian.PutUint32(b, value)
				dataBuf.Write(b)
			case uint64:
				b := make([]byte, 8)
				binary.BigEndian.PutUint64(b, value)
				dataBuf.Write(b)
			case []byte:
				dataBuf.Write(value)
			default:
				return fmt.Errorf("unsupported field type: %T", value)
			}
		}
	} else {
		payload, ok := mess.([]byte)
		if !ok {
			return errors.New("pass-through payload must be []byte")
		}
		dataBuf.Write(payload)
	}

	message.DataBuffer = dataBuf.Bytes()

	// Encrypt + compress payload when not passing through.
	if !isPass {
		compressed, err := crypto.GzipCompress(message.DataBuffer)
		if err != nil {
			return fmt.Errorf("gzip compress: %w", err)
		}
		encrypted, err := crypto.AESEncrypt(compressed, message.CryptoSecret)
		if err != nil {
			return fmt.Errorf("aes encrypt: %w", err)
		}
		message.DataBuffer = encrypted
	}

	// Build header.
	versionBuf := make([]byte, 2)
	messageTypeBuf := make([]byte, 2)
	routeLenBuf := make([]byte, 4)
	dataLenBuf := make([]byte, 8)

	binary.BigEndian.PutUint16(versionBuf, header.Version)
	if header.Version == 0 {
		binary.BigEndian.PutUint16(versionBuf, 1)
	}
	binary.BigEndian.PutUint16(messageTypeBuf, header.MessageType)
	binary.BigEndian.PutUint32(routeLenBuf, uint32(len(header.Route)))
	binary.BigEndian.PutUint64(dataLenBuf, uint64(len(message.DataBuffer)))

	headerBuf.Write(versionBuf)
	headerBuf.Write([]byte(padString(header.Sender, 10)))
	headerBuf.Write([]byte(padString(header.Accepter, 10)))
	headerBuf.Write(messageTypeBuf)
	headerBuf.Write(routeLenBuf)
	headerBuf.Write([]byte(header.Route))
	headerBuf.Write(dataLenBuf)

	message.HeaderBuffer = headerBuf.Bytes()
	return nil
}

// ConstructSuffix is a no-op for raw TCP.
func (message *RawMessage) ConstructSuffix() {}

// DeconstructHeader is a no-op; header is read in DeconstructData.
func (message *RawMessage) DeconstructHeader() {}

// DeconstructData reads the full message and deserializes it.
func (message *RawMessage) DeconstructData() (*Header, interface{}, error) {
	header := new(Header)

	versionBuf := make([]byte, 2)
	senderBuf := make([]byte, 10)
	accepterBuf := make([]byte, 10)
	messageTypeBuf := make([]byte, 2)
	routeLenBuf := make([]byte, 4)
	dataLenBuf := make([]byte, 8)

	if _, err := io.ReadFull(message.Conn, versionBuf); err != nil {
		return nil, nil, err
	}
	header.Version = binary.BigEndian.Uint16(versionBuf)

	if _, err := io.ReadFull(message.Conn, senderBuf); err != nil {
		return nil, nil, err
	}
	header.Sender = string(bytes.TrimRight(senderBuf, "\x00"))

	if _, err := io.ReadFull(message.Conn, accepterBuf); err != nil {
		return nil, nil, err
	}
	header.Accepter = string(bytes.TrimRight(accepterBuf, "\x00"))

	if _, err := io.ReadFull(message.Conn, messageTypeBuf); err != nil {
		return nil, nil, err
	}
	header.MessageType = binary.BigEndian.Uint16(messageTypeBuf)

	if _, err := io.ReadFull(message.Conn, routeLenBuf); err != nil {
		return nil, nil, err
	}
	header.RouteLen = binary.BigEndian.Uint32(routeLenBuf)

	routeBuf := make([]byte, header.RouteLen)
	if header.RouteLen > 0 {
		if _, err := io.ReadFull(message.Conn, routeBuf); err != nil {
			return nil, nil, err
		}
	}
	header.Route = string(routeBuf)

	if _, err := io.ReadFull(message.Conn, dataLenBuf); err != nil {
		return nil, nil, err
	}
	header.DataLen = binary.BigEndian.Uint64(dataLenBuf)

	dataBuf := make([]byte, header.DataLen)
	if header.DataLen > 0 {
		if _, err := io.ReadFull(message.Conn, dataBuf); err != nil {
			return nil, nil, err
		}
	}

	// Decrypt if the message is addressed to us, to temp, or is addressed
	// to the controller and we are the controller.
	// Relay traffic from children addressed to ADMIN_UUID stays encrypted
	// because intermediate nodes are not ADMIN_UUID.
	shouldDecrypt := header.Accepter == TEMP_UUID ||
		message.UUID == header.Accepter ||
		(message.UUID == ADMIN_UUID && header.Accepter == ADMIN_UUID)
	if !shouldDecrypt {
		return header, dataBuf, nil
	}

	decrypted, err := crypto.AESDecrypt(dataBuf, message.CryptoSecret)
	if err != nil {
		return nil, nil, fmt.Errorf("aes decrypt: %w", err)
	}
	decompressed, err := crypto.GzipDecompress(decrypted)
	if err != nil {
		return nil, nil, fmt.Errorf("gzip decompress: %w", err)
	}

	mess, err := unmarshalPayload(header.MessageType, decompressed)
	if err != nil {
		return nil, nil, err
	}

	return header, mess, nil
}

// DeconstructSuffix is a no-op for raw TCP.
func (message *RawMessage) DeconstructSuffix() {}

// SendMessage writes the full message to the connection.
func (message *RawMessage) SendMessage() error {
	if message.Conn == nil {
		return errors.New("no underlying connection")
	}
	final := append(message.HeaderBuffer, message.DataBuffer...)
	_, err := message.Conn.Write(final)
	message.HeaderBuffer = nil
	message.DataBuffer = nil
	return err
}

// padString pads or truncates a string to length n.
func padString(s string, n int) string {
	if len(s) >= n {
		return s[:n]
	}
	return s + string(make([]byte, n-len(s)))
}

// unmarshalPayload deserializes a payload into the concrete type for a message type.
func unmarshalPayload(msgType uint16, data []byte) (interface{}, error) {
	var mess interface{}
	switch msgType {
	case HI:
		mess = new(HIMess)
	case UUID:
		mess = new(UUIDMess)
	case CHILDUUIDREQ:
		mess = new(ChildUUIDReq)
	case CHILDUUIDRES:
		mess = new(ChildUUIDRes)
	case MYINFO:
		mess = new(MyInfo)
	case MYMEMO:
		mess = new(MyMemo)
	case LISTENREQ:
		mess = new(ListenReq)
	case LISTENRES:
		mess = new(ListenRes)
	case CONNECTSTART:
		mess = new(ConnectStart)
	case CONNECTDONE:
		mess = new(ConnectDone)
	case SOCKSSTART:
		mess = new(SocksStart)
	case SOCKSREADY:
		mess = new(SocksReady)
	case SOCKSTCPDATA:
		mess = new(SocksTCPData)
	case SOCKSUDPDATA:
		mess = new(SocksUDPData)
	case SOCKSTCPFIN:
		mess = new(SocksTCPFin)
	case FORWARDSTART:
		mess = new(ForwardStart)
	case FORWARDREADY:
		mess = new(ForwardReady)
	case FORWARDDATA:
		mess = new(ForwardData)
	case FORWARDFIN:
		mess = new(ForwardFin)
	case BACKWARDSTART:
		mess = new(BackwardStart)
	case BACKWARDREADY:
		mess = new(BackwardReady)
	case BACKWARDDATA:
		mess = new(BackwardData)
	case BACKWARDFIN:
		mess = new(BackWardFin)
	case FILESTATREQ:
		mess = new(FileStatReq)
	case FILESTATRES:
		mess = new(FileStatRes)
	case FILEDATA:
		mess = new(FileData)
	case FILEERR:
		mess = new(FileErr)
	case FILEDOWNREQ:
		mess = new(FileDownReq)
	case FILEDOWNRES:
		mess = new(FileDownRes)
	case NODEOFFLINE:
		mess = new(NodeOffline)
	case NODEREONLINE:
		mess = new(NodeReonline)
	case UPSTREAMOFFLINE:
		mess = new(UpstreamOffline)
	case UPSTREAMREONLINE:
		mess = new(UpstreamReonline)
	case SHUTDOWN:
		mess = new(Shutdown)
	case HEARTBEAT:
		mess = new(HeartbeatMsg)
	default:
		return nil, fmt.Errorf("unknown message type: %d", msgType)
	}

	val := reflect.ValueOf(mess).Elem()
	typ := val.Type()

	var ptr uint64
	for i := 0; i < typ.NumField(); i++ {
		field := val.Field(i)
		fieldName := typ.Field(i).Name

		switch field.Interface().(type) {
		case string:
			lenField := val.FieldByName(fieldName + "Len")
			if !lenField.IsValid() {
				return nil, fmt.Errorf("missing length field for %s", fieldName)
			}
			strLen := uint64Value(lenField)
			if ptr+strLen > uint64(len(data)) {
				return nil, fmt.Errorf("string field %s out of bounds", fieldName)
			}
			field.SetString(string(data[ptr : ptr+strLen]))
			ptr += strLen
		case uint16:
			if ptr+2 > uint64(len(data)) {
				return nil, fmt.Errorf("uint16 field %s out of bounds", fieldName)
			}
			field.SetUint(uint64(binary.BigEndian.Uint16(data[ptr : ptr+2])))
			ptr += 2
		case uint32:
			if ptr+4 > uint64(len(data)) {
				return nil, fmt.Errorf("uint32 field %s out of bounds", fieldName)
			}
			field.SetUint(uint64(binary.BigEndian.Uint32(data[ptr : ptr+4])))
			ptr += 4
		case uint64:
			if ptr+8 > uint64(len(data)) {
				return nil, fmt.Errorf("uint64 field %s out of bounds", fieldName)
			}
			field.SetUint(binary.BigEndian.Uint64(data[ptr : ptr+8]))
			ptr += 8
		case []byte:
			lenField := val.FieldByName(fieldName + "Len")
			if !lenField.IsValid() {
				return nil, fmt.Errorf("missing length field for %s", fieldName)
			}
			byteLen := uint64Value(lenField)
			if ptr+byteLen > uint64(len(data)) {
				return nil, fmt.Errorf("[]byte field %s out of bounds", fieldName)
			}
			field.SetBytes(data[ptr : ptr+byteLen])
			ptr += byteLen
		default:
			return nil, fmt.Errorf("unsupported field type in %s: %T", fieldName, field.Interface())
		}
	}

	return mess, nil
}

// uint64Value extracts a uint64 from a length field that may be uint16/uint32/uint64.
func uint64Value(v reflect.Value) uint64 {
	switch v.Interface().(type) {
	case uint16:
		return uint64(v.Uint())
	case uint32:
		return uint64(v.Uint())
	case uint64:
		return v.Uint()
	default:
		return v.Uint()
	}
}
