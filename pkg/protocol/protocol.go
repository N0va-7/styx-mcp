package protocol

import (
	"net"

	"mcp-stowaway/pkg/crypto"
)

// Upstream and Downstream transport types.
var (
	Upstream   string
	Downstream string
)

const (
	HI = iota
	UUID
	CHILDUUIDREQ
	CHILDUUIDRES
	MYINFO
	MYMEMO
	LISTENREQ
	LISTENRES
	CONNECTSTART
	CONNECTDONE
	SOCKSSTART
	SOCKSREADY
	SOCKSTCPDATA
	SOCKSUDPDATA
	SOCKSTCPFIN
	FORWARDTEST
	FORWARDSTART
	FORWARDREADY
	FORWARDDATA
	FORWARDFIN
	BACKWARDTEST
	BACKWARDSTART
	BACKWARDSEQ
	BACKWARDREADY
	BACKWARDDATA
	BACKWARDFIN
	BACKWARDSTOP
	BACKWARDSTOPDONE
	FILESTATREQ
	FILESTATRES
	FILEDATA
	FILEERR
	FILEDOWNREQ
	FILEDOWNRES
	NODEOFFLINE
	NODEREONLINE
	UPSTREAMOFFLINE
	UPSTREAMREONLINE
	SHUTDOWN
	HEARTBEAT
)

const (
	ADMIN_UUID = "IAMADMINXD"
	TEMP_UUID  = "IAMNEWHERE"
	TEMP_ROUTE = "THEREISNOROUTE"
)

// Proto is the transport negotiation interface.
type Proto interface {
	CNegotiate() error
	SNegotiate() error
}

// NegParam holds parameters for transport negotiation.
type NegParam struct {
	Domain string
	Conn   net.Conn
}

// Message is the application-layer message interface.
type Message interface {
	ConstructHeader()
	ConstructData(*Header, interface{}, bool) error
	ConstructSuffix()
	DeconstructHeader()
	DeconstructData() (*Header, interface{}, error)
	DeconstructSuffix()
	SendMessage() error
}

// ConstructMessage serializes header and payload.
func ConstructMessage(message Message, header *Header, mess interface{}, isPass bool) error {
	if err := message.ConstructData(header, mess, isPass); err != nil {
		return err
	}
	message.ConstructHeader()
	message.ConstructSuffix()
	return nil
}

// DestructMessage reads and deserializes a message.
func DestructMessage(message Message) (*Header, interface{}, error) {
	message.DeconstructHeader()
	header, mess, err := message.DeconstructData()
	if err != nil {
		return nil, nil, err
	}
	message.DeconstructSuffix()
	return header, mess, nil
}

// Header is the routing envelope for every message.
type Header struct {
	Version     uint16
	Sender      string
	Accepter    string
	MessageType uint16
	RouteLen    uint32
	Route       string
	DataLen     uint64
}

// HIMess is the initial handshake message.
type HIMess struct {
	GreetingLen uint16
	Greeting    string
	UUIDLen     uint16
	UUID        string
	IsAdmin     uint16
	IsReconnect uint16
}

// UUIDMess assigns a UUID to a node.
type UUIDMess struct {
	UUIDLen uint16
	UUID    string
}

// ChildUUIDReq requests a UUID for a new child node.
type ChildUUIDReq struct {
	ParentUUIDLen uint16
	ParentUUID    string
	IPLen         uint16
	IP            string
}

// ChildUUIDRes returns a UUID for a new child node.
type ChildUUIDRes struct {
	UUIDLen uint16
	UUID    string
}

// MyInfo carries node system information.
type MyInfo struct {
	UUIDLen     uint16
	UUID        string
	UsernameLen uint64
	Username    string
	HostnameLen uint64
	Hostname    string
	MemoLen     uint64
	Memo        string
}

// MyMemo updates a node memo.
type MyMemo struct {
	MemoLen uint64
	Memo    string
}

// ListenReq asks a node to start listening for child connections.
type ListenReq struct {
	Method  uint16
	AddrLen uint64
	Addr    string
}

// ListenRes confirms a listen operation.
type ListenRes struct {
	OK uint16
}

// ConnectStart instructs a node to connect to a new child.
type ConnectStart struct {
	AddrLen uint16
	Addr    string
}

// ConnectDone confirms a connect operation.
type ConnectDone struct {
	OK uint16
}

// SocksStart starts a SOCKS5 proxy service.
type SocksStart struct {
	AddrLen uint16
	Addr    string
}

// SocksReady confirms SOCKS5 startup.
type SocksReady struct {
	OK uint16
}

// SocksTCPData carries TCP data for a SOCKS stream.
type SocksTCPData struct {
	Seq     uint64
	DataLen uint64
	Data    []byte
}

// SocksUDPData carries UDP data for a SOCKS stream.
type SocksUDPData struct {
	Seq     uint64
	DataLen uint64
	Data    []byte
}

// SocksTCPFin signals the end of a SOCKS TCP stream.
type SocksTCPFin struct {
	Seq uint64
}

// ForwardStart starts a port forward.
type ForwardStart struct {
	Seq     uint64
	AddrLen uint16
	Addr    string
}

// ForwardReady confirms a forward setup.
type ForwardReady struct {
	OK uint16
}

// ForwardData carries forwarded data.
type ForwardData struct {
	Seq     uint64
	DataLen uint64
	Data    []byte
}

// ForwardFin signals the end of a forward stream.
type ForwardFin struct {
	Seq uint64
}

// BackwardStart starts a reverse port forward.
type BackwardStart struct {
	UUIDLen  uint16
	UUID     string
	LPortLen uint16
	LPort    string
	RPortLen uint16
	RPort    string
}

// BackwardReady confirms reverse forward setup.
type BackwardReady struct {
	OK uint16
}

// BackwardSeq assigns a sequence number to a reverse forward connection.
type BackwardSeq struct {
	Seq      uint64
	RPortLen uint16
	RPort    string
}

// BackwardData carries reverse forwarded data.
type BackwardData struct {
	Seq     uint64
	DataLen uint64
	Data    []byte
}

// BackWardFin signals the end of a reverse forward stream.
type BackWardFin struct {
	Seq uint64
}

// BackwardStop requests stopping a reverse forward.
type BackwardStop struct {
	All      uint16
	RPortLen uint16
	RPort    string
}

// BackwardStopDone confirms reverse forward stop.
type BackwardStopDone struct {
	All     uint16
	UUIDLen uint16
	UUID    string
}

// FileStatReq initiates a file transfer.
type FileStatReq struct {
	FilenameLen uint32
	Filename    string
	FileSize    uint64
	SliceNum    uint64
}

// FileStatRes confirms a file transfer.
type FileStatRes struct {
	OK uint16
}

// FileData carries a file chunk.
type FileData struct {
	DataLen uint64
	Data    []byte
}

// FileErr reports a file transfer error.
type FileErr struct {
	Error uint16
}

// FileDownReq requests a file download.
type FileDownReq struct {
	FilePathLen uint32
	FilePath    string
	FilenameLen uint32
	Filename    string
}

// FileDownRes confirms a file download request.
type FileDownRes struct {
	OK uint16
}

// NodeOffline notifies that a node went offline.
type NodeOffline struct {
	UUIDLen uint16
	UUID    string
}

// NodeReonline notifies that a node reconnected.
type NodeReonline struct {
	ParentUUIDLen uint16
	ParentUUID    string
	UUIDLen       uint16
	UUID          string
	IPLen         uint16
	IP            string
}

// UpstreamOffline notifies that the upstream connection is lost.
type UpstreamOffline struct {
	OK uint16
}

// UpstreamReonline notifies that the upstream connection is restored.
type UpstreamReonline struct {
	OK uint16
}

// Shutdown terminates a node.
type Shutdown struct {
	OK uint16
}

// HeartbeatMsg is a keepalive ping.
type HeartbeatMsg struct {
	Ping uint16
}

// MessageComponent holds per-connection state.
type MessageComponent struct {
	UUID   string
	Conn   net.Conn
	Secret string
}

// SetUpDownStream configures transport types.
func SetUpDownStream(upstream, downstream string) {
	if upstream == "ws" {
		Upstream = "ws"
	} else {
		Upstream = "raw"
	}

	if downstream == "ws" {
		Downstream = "ws"
	} else {
		Downstream = "raw"
	}
}

// NewUpProto creates an upstream transport negotiator.
func NewUpProto(param *NegParam) Proto {
	switch Upstream {
	case "ws":
		return &WSProto{domain: param.Domain, conn: param.Conn}
	default:
		return &RawProto{}
	}
}

// NewDownProto creates a downstream transport negotiator.
func NewDownProto(param *NegParam) Proto {
	switch Downstream {
	case "ws":
		return &WSProto{domain: param.Domain, conn: param.Conn}
	default:
		return &RawProto{}
	}
}

// NewUpMsg creates an upstream message encoder/decoder.
func NewUpMsg(conn net.Conn, secret, uuid string) Message {
	key := crypto.KeyPadding([]byte(secret))
	switch Upstream {
	case "ws":
		return &WSMessage{RawMessage: &RawMessage{Conn: conn, UUID: uuid, CryptoSecret: key}}
	default:
		return &RawMessage{Conn: conn, UUID: uuid, CryptoSecret: key}
	}
}

// NewDownMsg creates a downstream message encoder/decoder.
func NewDownMsg(conn net.Conn, secret, uuid string) Message {
	key := crypto.KeyPadding([]byte(secret))
	switch Downstream {
	case "ws":
		return &WSMessage{RawMessage: &RawMessage{Conn: conn, UUID: uuid, CryptoSecret: key}}
	default:
		return &RawMessage{Conn: conn, UUID: uuid, CryptoSecret: key}
	}
}
