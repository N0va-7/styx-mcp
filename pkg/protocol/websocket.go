package protocol

import (
	"errors"
	"net"
)

// WSProto is a stub: WebSocket transport is not implemented.
// CLI rejects -up/-down ws; these methods remain for switch completeness.
type WSProto struct {
	domain string
	conn   net.Conn
}

// CNegotiate performs the client-side WebSocket handshake.
func (proto *WSProto) CNegotiate() error {
	return errors.New("websocket transport not implemented; use raw")
}

// SNegotiate performs the server-side WebSocket handshake.
func (proto *WSProto) SNegotiate() error {
	return errors.New("websocket transport not implemented; use raw")
}

// WSMessage is a placeholder WebSocket message wrapper.
type WSMessage struct {
	*RawMessage
}

// ConstructData delegates to the embedded RawMessage.
func (message *WSMessage) ConstructData(header *Header, mess interface{}, isPass bool) error {
	return message.RawMessage.ConstructData(header, mess, isPass)
}

// ConstructHeader delegates to the embedded RawMessage.
func (message *WSMessage) ConstructHeader() {
	message.RawMessage.ConstructHeader()
}

// ConstructSuffix delegates to the embedded RawMessage.
func (message *WSMessage) ConstructSuffix() {
	message.RawMessage.ConstructSuffix()
}

// DeconstructHeader delegates to the embedded RawMessage.
func (message *WSMessage) DeconstructHeader() {
	message.RawMessage.DeconstructHeader()
}

// DeconstructData delegates to the embedded RawMessage.
func (message *WSMessage) DeconstructData() (*Header, interface{}, error) {
	return message.RawMessage.DeconstructData()
}

// DeconstructSuffix delegates to the embedded RawMessage.
func (message *WSMessage) DeconstructSuffix() {
	message.RawMessage.DeconstructSuffix()
}

// SendMessage delegates to the embedded RawMessage.
func (message *WSMessage) SendMessage() error {
	return message.RawMessage.SendMessage()
}
