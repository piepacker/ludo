package network

import (
	"unsafe"

	"github.com/libretro/ludo/ggpo/ggponet"
)

const (
	MAX_COMPRESSED_BITS = 4096
	MSG_MAX_PLAYERS     = 4
)

type MsgType int64

const (
	Invalid MsgType = iota
	SyncRequest
	SyncReply
	Input
	QualityReport
	QualityReply
	KeepAlive
	InputAck
)

type HdrType struct {
	Magic          uint64
	SequenceNumber uint64
	Type           MsgType
}

type SyncRequestType struct {
	RandomRequest   uint64 /* please reply back with this random data */
	RemoteMagic     int64
	RemoteEndpoints int64
}
type SyncReplyType struct {
	RandomReply uint64 /* OK, here's your random data back */
}

type QualityReportType struct {
	FrameAdvantage int64 /* what's the other guy's frame advantage? */
	Ping           int64
}

type QualityReplyType struct {
	Pong int64
}

type InputType struct {
	PeerConnectStatus   []ggponet.ConnectStatus
	StartFrame          int64
	DisconnectRequested bool
	AckFrame            int64
	NumBits             int64
	InputSize           int64  // XXX: shouldn't be in every single packet!
	Bits                []byte /* must be last */
}

type InputAckType struct {
	AckFrame int64
}

type NetplayMsgType struct {
	ConnectStatus ggponet.ConnectStatus
	Hdr           HdrType
	SyncRequest   SyncRequestType
	SyncReply     SyncReplyType
	QualityReport QualityReportType
	QualityReply  QualityReplyType
	Input         InputType
	InputAck      InputAckType
}

type StateType struct {
	Sync    SyncType
	Running RunningType
}

type SyncType struct {
	RoundTripsRemaining uint64
	Random              uint64
}

type RunningType struct {
	LastQualityReportTime    uint64
	LastNetworkStatsInterval uint64
	LastInputPacketRecvTime  uint64
}

func (n *NetplayMsgType) Init(t MsgType) {
	n.Hdr.Type = t
}

func (n *NetplayMsgType) PacketSize() int64 {
	return int64(unsafe.Sizeof(n.Hdr)) + n.PayloadSize()
}

func (n *NetplayMsgType) PayloadSize() int64 {
	switch n.Hdr.Type {
	case SyncRequest:
		return int64(unsafe.Sizeof(n.SyncRequest))
	case SyncReply:
		return int64(unsafe.Sizeof(n.SyncReply))
	case QualityReport:
		return int64(unsafe.Sizeof(n.QualityReport))
	case QualityReply:
		return int64(unsafe.Sizeof(n.QualityReply))
	case InputAck:
		return int64(unsafe.Sizeof(n.InputAck))
	case KeepAlive:
		return 0
	case Input:
		return int64(unsafe.Sizeof(n.Input))
	}
	return 0
}
