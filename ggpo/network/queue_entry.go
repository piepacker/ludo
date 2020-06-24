package network

import "net"

type QueueEntry struct {
	QueueTime uint64
	DestAddr  *net.UDPAddr
	Msg       *NetplayMsgType
}

func (q *QueueEntry) Init(time uint64, dst *net.UDPAddr, m *NetplayMsgType) {
	q.QueueTime = time
	q.DestAddr = dst
	q.Msg = m
}
