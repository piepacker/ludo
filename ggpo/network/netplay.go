package network

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"math/rand"
	"net"

	"github.com/libretro/ludo/ggpo/bitvector"
	"github.com/libretro/ludo/ggpo/ggponet"
	"github.com/libretro/ludo/ggpo/lib"
	"github.com/libretro/ludo/ggpo/platform"
	"github.com/sirupsen/logrus"
)

type OoPacket struct {
	SendTime uint64
	DestAddr *net.UDPAddr
	Msg      *NetplayMsgType
}

type State int64

const (
	Syncing State = iota
	Synchronzied
	Running
	Disconnected
)

const (
	UDP_HEADER_SIZE           = 28 /* Size of IP + UDP headers */
	NUM_SYNC_PACKETS          = 5
	SYNC_RETRY_INTERVAL       = 2000
	SYNC_FIRST_RETRY_INTERVAL = 500
	RUNNING_RETRY_INTERVAL    = 200
	KEEP_ALIVE_INTERVAL       = 200
	QUALITY_REPORT_INTERVAL   = 1000
	NETWORK_STATS_INTERVAL    = 1000
	UDP_SHUTDOWN_TIMER        = 5000
	MAX_SEQ_DISTANCE          = (1 << 15)
)

type Netplay struct {
	Callbacks             ggponet.GGPOSessionCallbacks
	Conn                  *net.UDPConn
	LocalAddr             *net.UDPAddr
	RemoteAddr            *net.UDPAddr
	Queue                 int64
	IsHosting             bool
	LastReceivedInput     lib.GameInput
	LastAckedInput        lib.GameInput
	LastSentInput         lib.GameInput
	LocalConnectStatus    []ggponet.ConnectStatus
	LocalFrameAdvantage   int64
	RemoteFrameAdvantage  int64
	RoundTripTime         int64
	PeerConnectStatus     []ggponet.ConnectStatus
	TimeSync              lib.TimeSync
	CurrentState          State
	PendingOutput         lib.RingBuffer
	LastSendTime          uint64
	MagicNumber           uint64
	RemoteMagicNumber     uint64
	Connected             bool
	NextSendSeq           uint64
	SendQueue             lib.RingBuffer
	EventQueue            lib.RingBuffer
	SendLatency           int64
	OopPercent            int64
	OoPacket              OoPacket
	NetplayState          StateType
	ReceiveChannel        chan NetplayMsgType
	DisconnectNotifyStart int64
	DisconnectNotifySent  bool
	NextRecvSeq           uint64
	DisconnectTimeout     int64
	DisconnectEventSent   bool
	LastRecvTime          uint64
	ShutDownTimeout       int64
	StatsStartTime        uint64
	KbpsSent              int64
	BytesSent             int64
	PacketsSent           int64
	IsInitialized         bool
}

func (n *Netplay) Init(remotePlayer ggponet.GGPOPlayer, localPort string, queue int64, status []ggponet.ConnectStatus, poll *lib.Poll) {
	n.LocalAddr, _ = net.ResolveUDPAddr("udp4", fmt.Sprintf("127.0.0.1:%s", localPort))
	n.RemoteAddr, _ = net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", remotePlayer.IPAddress, int(remotePlayer.Port)))
	n.Queue = queue
	n.IsInitialized = false
	n.LastReceivedInput.SimpleInit(-1, nil, 1)
	n.LastAckedInput.SimpleInit(-1, nil, 1)
	n.LastSentInput.SimpleInit(-1, nil, 1)
	n.LocalConnectStatus = status
	n.LocalFrameAdvantage = 0
	n.PendingOutput.Init(64)
	n.PacketsSent = 0
	n.BytesSent = 0
	n.LastSendTime = 0
	n.NextSendSeq = 0
	n.MagicNumber = 0
	n.DisconnectNotifyStart = 0
	n.DisconnectNotifySent = false
	n.NextRecvSeq = 0
	n.DisconnectTimeout = 0
	n.DisconnectEventSent = false
	n.LastRecvTime = 0
	n.ShutDownTimeout = 0
	n.StatsStartTime = 0
	n.KbpsSent = 0
	n.BytesSent = 0
	n.PacketsSent = 0
	n.SendQueue.Init(64)
	n.EventQueue.Init(64)
	for n.MagicNumber == 0 {
		n.MagicNumber = rand.Uint64()
	}
	n.PeerConnectStatus = make([]ggponet.ConnectStatus, MSG_MAX_PLAYERS)
	for i := 0; i < len(n.PeerConnectStatus); i++ {
		n.PeerConnectStatus[i].LastFrame = -1
	}

	n.SendLatency = platform.GetConfigInt("ggpo.network.delay") //TODO: Really usefull???
	n.OopPercent = platform.GetConfigInt("ggpo.oop.percent")
	n.OoPacket.Msg = nil

	var i lib.IPollSink = n
	poll.RegisterLoop(&i)

	n.ReceiveChannel = make(chan NetplayMsgType, 60)

	logrus.Info(fmt.Sprintf("binding udp socket to port %d.", n.LocalAddr.Port))
}

func (n *Netplay) Write(msg *NetplayMsgType) {
	var err error
	var buffer bytes.Buffer
	encoder := gob.NewEncoder(&buffer)
	encoder.Encode(msg)
	if n.IsHosting {
		_, err = n.Conn.WriteToUDP(buffer.Bytes(), n.RemoteAddr)
	} else {
		_, err = n.Conn.Write(buffer.Bytes())
	}
	if err != nil {
		logrus.Error("Netplay Write Error : ", err)
		return
	}
}

func (n *Netplay) Read() {
	var msg *NetplayMsgType = new(NetplayMsgType)
	for {
		netinput := make([]byte, 8192)
		length, _, err := n.Conn.ReadFromUDP(netinput)
		if err != nil {
			logrus.Error("Netplay Read Error : ", err)
			logrus.Error("Netplay Length : ", length)
			return
		}
		buffer := bytes.NewBuffer(netinput[:length])
		decoder := gob.NewDecoder(buffer)
		decoder.Decode(msg)
		n.ReceiveChannel <- *msg
	}
}

func (n *Netplay) SendGameState(state lib.SavedFrame) {
	var msg *NetplayMsgType = new(NetplayMsgType)
	msg.Init(GameState)
	msg.GameState.Checksum = state.Checksum
	msg.GameState.StartFrame = state.Frame
	if n.LocalConnectStatus != nil {
		copy(msg.GameState.PeerConnectStatus, n.LocalConnectStatus)
	} else {
		msg.GameState.PeerConnectStatus = make([]ggponet.ConnectStatus, MSG_MAX_PLAYERS)
	}
	n.SendMsg(msg)
}

func (n *Netplay) SendInput(input *lib.GameInput) {
	if n.CurrentState == Running {
		n.TimeSync.AdvanceFrame(input, n.LocalFrameAdvantage, n.RemoteFrameAdvantage)
		var t lib.T = input
		n.PendingOutput.Push(&t)
	}
	n.SendPendingOutput()
}

func (n *Netplay) SendPendingOutput() {
	var msg *NetplayMsgType = new(NetplayMsgType)
	msg.Init(Input)
	offset := int64(0)
	var bits []byte
	var last lib.GameInput

	if n.PendingOutput.Size > 0 {
		last = n.LastAckedInput
		msg.Input.Bits = make([]byte, MAX_COMPRESSED_BITS)
		bits = msg.Input.Bits

		var input *lib.GameInput = (*n.PendingOutput.Front()).(*lib.GameInput)
		msg.Input.StartFrame = input.Frame
		msg.Input.InputSize = input.Size

		if last.Frame != -1 && last.Frame+1 != msg.Input.StartFrame {
			logrus.Panic("Assert Error last.Frame != -1 && last.Frame+1 != msg.Input.StartFrame. last.Frame = ", last.Frame, " msg.Input.StartFrame = ", msg.Input.StartFrame)
		}
		for j := int64(0); j < n.PendingOutput.Size; j++ {
			current := (*n.PendingOutput.Item(j)).(*lib.GameInput)
			if bytes.Compare(current.Bits, last.Bits) != 0 {
				if (lib.GAMEINPUT_MAX_BYTES * lib.GAMEINPUT_MAX_PLAYERS * 8) >= (1 << bitvector.BITVECTOR_NIBBLE_SIZE) {
					logrus.Panic("Assert Error (lib.GAMEINPUT_MAX_BYTES * lib.GAMEINPUT_MAX_PLAYERS * 8) >= (1 << bitvector.BITVECTOR_NIBBLE_SIZE)")
				}
				for i := int64(0); i < current.Size*8; i++ {
					if i >= (1 << bitvector.BITVECTOR_NIBBLE_SIZE) {
						logrus.Panic("Assert Error i >= (1 << bitvector.BITVECTOR_NIBBLE_SIZE)")
					}
					if current.Value(i) != last.Value(i) {
						bitvector.SetBit(msg.Input.Bits, &offset)
						if current.Value(i) {
							bitvector.SetBit(bits, &offset)
						} else {
							bitvector.ClearBit(bits, &offset)
						}
						bitvector.WriteNibblet(bits, i, &offset)
					}
				}
			}
			bitvector.ClearBit(msg.Input.Bits, &offset)
			last = *current
			n.LastSentInput = *current
		}
	} else {
		msg.Input.StartFrame = 0
		msg.Input.InputSize = 0
	}
	msg.Input.AckFrame = n.LastReceivedInput.Frame
	msg.Input.NumBits = offset

	msg.Input.DisconnectRequested = n.CurrentState == Disconnected
	if n.LocalConnectStatus != nil {
		copy(msg.Input.PeerConnectStatus, n.LocalConnectStatus)
	} else {
		msg.Input.PeerConnectStatus = make([]ggponet.ConnectStatus, MSG_MAX_PLAYERS)
	}

	n.SendMsg(msg)
}

func (n *Netplay) SendMsg(msg *NetplayMsgType) {
	n.PacketsSent++
	n.LastSendTime = platform.GetCurrentTimeMS()
	n.BytesSent += msg.PacketSize()

	msg.Hdr.Magic = n.MagicNumber
	msg.Hdr.SequenceNumber = n.NextSendSeq
	n.NextSendSeq++

	var queue QueueEntry
	queue.Init(platform.GetCurrentTimeMS(), n.RemoteAddr, msg)
	var t lib.T = &queue
	n.SendQueue.Push(&t)
	n.PumpSendQueue()
}

func (n *Netplay) HostConnection(conn *net.UDPConn) *net.UDPConn {
	logrus.Info("Netplay HostConnection")
	n.IsHosting = true
	if conn == nil {
		var err error
		n.Conn, err = net.ListenUDP("udp4", n.LocalAddr)
		if err != nil {
			logrus.Error("HostConnection Error : ", err)
			return nil
		}
	} else {
		n.Conn = conn
	}
	n.IsInitialized = true
	go n.Read()
	return n.Conn
}

func (n *Netplay) JoinConnection() {
	logrus.Info("Netplay JoinConnection")
	n.IsHosting = false
	var err error
	n.Conn, err = net.DialUDP("udp4", n.LocalAddr, n.RemoteAddr)
	if err != nil {
		logrus.Error("JoinConnection Error : ", err)
		return
	}
	n.IsInitialized = true
	go n.Read()
}

func (n *Netplay) Disconnect() ggponet.GGPOErrorCode {
	logrus.Info("Netplay Disconnect !")
	n.CurrentState = Disconnected
	n.ShutDownTimeout = int64(platform.GetCurrentTimeMS()) + UDP_SHUTDOWN_TIMER
	if n.Conn == nil {
		return ggponet.GGPO_OK
	}
	return ggponet.GGPO_ERRORCODE_PLAYER_DISCONNECTED
}

func (n *Netplay) GetPeerConnectStatus(id int64, frame *int64) bool {
	logrus.Info("LastFrame ", n.PeerConnectStatus[id].LastFrame, ", id ", id)
	*frame = n.PeerConnectStatus[id].LastFrame
	return !n.PeerConnectStatus[id].Disconnected
}

func (n *Netplay) OnSyncRequest(msg *NetplayMsgType) bool {
	if n.RemoteMagicNumber != 0 && msg.Hdr.Magic != n.RemoteMagicNumber {
		logrus.Info(fmt.Sprintf("Ignoring sync request from unknown endpoint (%d != %d).", msg.Hdr.Magic, n.RemoteMagicNumber))
		return false
	}
	var reply *NetplayMsgType = new(NetplayMsgType)
	reply.Init(SyncReply)
	reply.SyncReply.RandomReply = msg.SyncRequest.RandomRequest
	n.SendMsg(reply)
	return true
}

func (n *Netplay) OnSyncReply(msg *NetplayMsgType) bool {
	if n.CurrentState != Syncing {
		logrus.Info("Ignoring SyncReply while not synching.")
		return msg.Hdr.Magic == n.RemoteMagicNumber
	}

	if msg.SyncReply.RandomReply != n.NetplayState.Sync.Random {
		logrus.Info(fmt.Sprintf("sync reply %d != %d.  Keep looking...", msg.SyncReply.RandomReply, n.NetplayState.Sync.Random))
		return false
	}

	if !n.Connected {
		var evt Event
		evt.Init(EventConnected)
		n.QueueEvent(&evt)
		n.Connected = true
	}

	logrus.Info(fmt.Sprintf("Checking sync state (%d round trips remaining).", n.NetplayState.Sync.RoundTripsRemaining))
	n.NetplayState.Sync.RoundTripsRemaining--
	if n.NetplayState.Sync.RoundTripsRemaining == 0 {
		logrus.Info("Synchronized!")
		var evt Event
		evt.Init(EventSynchronized)
		n.QueueEvent(&evt)
		n.CurrentState = Running
		n.LastReceivedInput.Frame = -1
		n.RemoteMagicNumber = msg.Hdr.Magic
	} else {
		var evt Event
		evt.Init(EventSynchronizing)
		evt.Synchronizing.Total = NUM_SYNC_PACKETS
		evt.Synchronizing.Count = NUM_SYNC_PACKETS - int64(n.NetplayState.Sync.RoundTripsRemaining)
		n.QueueEvent(&evt)
		n.SendSyncRequest()
	}
	return true
}

func (n *Netplay) OnGameState(msg *NetplayMsgType) bool {
	logrus.Info(fmt.Sprintf("Received game state (checksum: %08x).", msg.GameState.Checksum))
	// Update the peer connection status if this peer is still considered to be part of the network.
	remoteStatus := msg.GameState.PeerConnectStatus
	for i := 0; i < len(remoteStatus); i++ {
		if remoteStatus[i].LastFrame < n.PeerConnectStatus[i].LastFrame {
			logrus.Panic("Assert error remotestatus Lastframe")
		}
		n.PeerConnectStatus[i].Disconnected = n.PeerConnectStatus[i].Disconnected || remoteStatus[i].Disconnected
		n.PeerConnectStatus[i].LastFrame = lib.MAX(n.PeerConnectStatus[i].LastFrame, remoteStatus[i].LastFrame)
	}

	// Send the event to the emulator
	var evt Event
	evt.Init(EventGameState)
	evt.SavedFrame.Checksum = msg.GameState.Checksum
	evt.SavedFrame.Frame = msg.GameState.StartFrame
	n.QueueEvent(&evt)

	return true
}

func (n *Netplay) OnInput(msg *NetplayMsgType) bool {
	// If a disconnect is requested, go ahead and disconnect now.
	disconnectRequested := msg.Input.DisconnectRequested
	if disconnectRequested {
		if n.CurrentState != Disconnected && !n.DisconnectEventSent {
			logrus.Info("Disconnecting endpoint on remote request.")
			var evt Event
			evt.Init(EventDisconnected)
			n.QueueEvent(&evt)
			n.DisconnectEventSent = true
		}
	} else {
		// Update the peer connection status if this peer is still considered to be part of the network.
		remoteStatus := msg.Input.PeerConnectStatus
		for i := 0; i < len(remoteStatus); i++ {
			if remoteStatus[i].LastFrame < n.PeerConnectStatus[i].LastFrame {
				logrus.Panic("Assert error remotestatus Lastframe")
			}
			n.PeerConnectStatus[i].Disconnected = n.PeerConnectStatus[i].Disconnected || remoteStatus[i].Disconnected
			n.PeerConnectStatus[i].LastFrame = lib.MAX(n.PeerConnectStatus[i].LastFrame, remoteStatus[i].LastFrame)
		}
	}

	// Decompress the input.

	lastReceivedFrameNumber := n.LastReceivedInput.Frame
	if msg.Input.NumBits > 0 {
		var offset int64 = 0
		bits := msg.Input.Bits
		numBits := msg.Input.NumBits
		currentFrame := msg.Input.StartFrame

		n.LastReceivedInput.Size = msg.Input.InputSize
		if n.LastReceivedInput.Frame < 0 {
			n.LastReceivedInput.Frame = msg.Input.StartFrame - 1
		}

		for offset < numBits {
			/*
			* Keep walking through the frames (parsing bits) until we reach
			* the inputs for the frame right after the one we're on.
			 */

			if currentFrame > n.LastReceivedInput.Frame+1 {
				logrus.Panic("Assert error currentframe : ", currentFrame, " > n.LastReceivedInput.Frame+1 : ", n.LastReceivedInput.Frame+1)
			}
			var useInputs bool = currentFrame == n.LastReceivedInput.Frame+1

			for bitvector.ReadBit(bits, &offset) > 0 {
				on := bitvector.ReadBit(bits, &offset)
				button := bitvector.ReadNibblet(bits, &offset)
				if useInputs {
					if on > 0 {
						logrus.Info("Button : ", bits) //TODO: It has been temporary fixed %40
						n.LastReceivedInput.Set(button)
					} else {
						n.LastReceivedInput.Clear(button)
					}
				}
			}
			logrus.Info("Button Input : ", n.LastReceivedInput)
			// if offset > numBits { //TODO: Fix
			// 	logrus.Panic("Assert error offset : ", offset, " > numBits : ", numBits)
			// }

			// Now if we want to use these inputs, go ahead and send them to the emulator.
			if useInputs {
				// Move forward 1 frame in the stream.
				if currentFrame != n.LastReceivedInput.Frame+1 {
					logrus.Panic("Assert error currentframe : ", currentFrame, " != n.LastReceivedInput.Frame+1 : ", n.LastReceivedInput.Frame+1)
				}
				n.LastReceivedInput.Frame = currentFrame

				// Send the event to the emulator
				var evt Event
				evt.Init(EventInput)
				evt.Input = n.LastReceivedInput
				n.QueueEvent(&evt)

				n.NetplayState.Running.LastInputPacketRecvTime = platform.GetCurrentTimeMS()
				logrus.Info(fmt.Sprintf("Sending frame %d to emu queue %d", n.LastReceivedInput.Frame, n.Queue))
			} else {
				logrus.Info(fmt.Sprintf("Skipping past frame:(%d) current is %d.", currentFrame, n.LastReceivedInput.Frame))
			}

			// Move forward 1 frame in the input stream.
			currentFrame++
		}
	}
	if n.LastReceivedInput.Frame < lastReceivedFrameNumber {
		logrus.Panic("Assert error last received frame number")
	}

	// Get rid of our buffered input

	for n.PendingOutput.Size > 0 && (*n.PendingOutput.Front()).(*lib.GameInput).Frame < msg.Input.AckFrame {
		logrus.Info(fmt.Sprintf("Throwing away pending output frame %d", (*n.PendingOutput.Front()).(*lib.GameInput).Frame))
		n.LastAckedInput = *(*n.PendingOutput.Front()).(*lib.GameInput)
		n.PendingOutput.Pop()
	}
	return true
}

func (n *Netplay) OnInputAck(msg *NetplayMsgType) bool {
	// Get rid of our buffered input
	for n.PendingOutput.Size > 0 && (*n.PendingOutput.Front()).(lib.GameInput).Frame < msg.InputAck.AckFrame {
		logrus.Info(fmt.Sprintf("Throwing away pending output frame %d", (*n.PendingOutput.Front()).(lib.GameInput).Frame))
		n.LastAckedInput = (*n.PendingOutput.Front()).(lib.GameInput)
		n.PendingOutput.Pop()
	}
	return true
}

func (n *Netplay) OnQualityReport(msg *NetplayMsgType) bool {
	// send a reply so the other side can compute the round trip transmit time.
	var reply *NetplayMsgType = new(NetplayMsgType)
	reply.Init(QualityReply)
	reply.QualityReply.Pong = msg.QualityReport.Ping
	n.SendMsg(reply)

	n.RemoteFrameAdvantage = msg.QualityReport.FrameAdvantage
	return true
}

func (n *Netplay) OnQualityReply(msg *NetplayMsgType) bool {
	n.RoundTripTime = int64(platform.GetCurrentTimeMS()) - msg.QualityReply.Pong
	return true
}

func (n *Netplay) OnKeepAlive(msg *NetplayMsgType) bool {
	return true
}

func (n *Netplay) GetNetworkStats(s *ggponet.GGPONetworkStats) {
	s.Network.Ping = n.RoundTripTime
	s.Network.SendQueueLen = n.PendingOutput.Size
	s.Network.KbpsSent = n.KbpsSent
	s.TimeSync.RemoteFramesBehind = n.RemoteFrameAdvantage
	s.TimeSync.LocalFramesBehind = n.LocalFrameAdvantage
}

func (n *Netplay) SetLocalFrameNumber(localFrame int64) {
	remoteFrame := n.LastReceivedInput.Frame + (n.RoundTripTime * 60 / 1000)
	n.LocalFrameAdvantage = remoteFrame - localFrame
}

func (n *Netplay) RecommendFrameDelay() int64 {
	return n.TimeSync.RecommendFrameWaitDuration(false)
}

func (n *Netplay) PumpSendQueue() {
	for !n.SendQueue.Empty() {
		var entry *QueueEntry = (*n.SendQueue.Front()).(*QueueEntry)

		if n.SendLatency > 0 {
			// should really come up with a gaussian distributation based on the configured
			// value, but this will do for now.
			jitter := (n.SendLatency * 2 / 3) + ((rand.Int63() % n.SendLatency) / 3)
			if platform.GetCurrentTimeMS() < ((*n.SendQueue.Front()).(QueueEntry).QueueTime + uint64(jitter)) {
				break
			}
		}
		if n.OopPercent > 0 && n.OoPacket.Msg == nil && ((rand.Int63() % 100) < n.OopPercent) {
			delay := rand.Int63() % (n.SendLatency*10 + 1000)
			logrus.Info(fmt.Sprintf("creating rogue oop (seq: %d  delay: %d)", entry.Msg.Hdr.SequenceNumber, delay))
			n.OoPacket.SendTime = platform.GetCurrentTimeMS() + uint64(delay)
			n.OoPacket.Msg = entry.Msg
			n.OoPacket.DestAddr = entry.DestAddr
		} else {
			n.Write(entry.Msg)
			entry.Msg = nil
		}
		n.SendQueue.Pop()
	}
	if n.OoPacket.Msg != nil && n.OoPacket.SendTime < platform.GetCurrentTimeMS() {
		logrus.Info("sending rogue oop!")
		n.Write(n.OoPacket.Msg)
		n.OoPacket.Msg = nil
	}
}

func (n *Netplay) SetDisconnectNotifyStart(timeout int64) {
	n.DisconnectNotifyStart = timeout
}

func (n *Netplay) SetDisconnectTimeout(timeout int64) {
	n.DisconnectTimeout = timeout
}

func (n *Netplay) GetEvent(e *Event) bool {
	if n.EventQueue.Size == 0 {
		return false
	}
	e.Type = (*n.EventQueue.Front()).(*Event).Type
	e.Synchronizing = (*n.EventQueue.Front()).(*Event).Synchronizing
	e.Input = (*n.EventQueue.Front()).(*Event).Input
	e.DisconnectTimeout = (*n.EventQueue.Front()).(*Event).DisconnectTimeout
	n.EventQueue.Pop()
	return true
}

func (n *Netplay) QueueEvent(e *Event) {
	var t lib.T = e
	n.EventQueue.Push(&t)
}

func (n *Netplay) SendSyncRequest() {
	n.NetplayState.Sync.Random = rand.Uint64() & 0xFFFF
	var msg *NetplayMsgType = new(NetplayMsgType)
	msg.Init(SyncRequest)
	msg.SyncRequest.RandomRequest = n.NetplayState.Sync.Random
	n.SendMsg(msg)
}

func (n *Netplay) UpdateNetworkStats() {
	now := platform.GetCurrentTimeMS()

	if n.StatsStartTime == 0 {
		n.StatsStartTime = now
	}
	totalBytesSent := n.BytesSent + (UDP_HEADER_SIZE * n.PacketsSent)
	seconds := float64((now - n.StatsStartTime) / 1000.0)
	Bps := float64(totalBytesSent) / seconds
	udpOverhead := float64(100.0 * (UDP_HEADER_SIZE * n.PacketsSent) / n.BytesSent)

	n.KbpsSent = int64(Bps / 1024)

	if n.StatsStartTime != now {
		logrus.Info(fmt.Sprintf("Network Stats -- Bandwidth: %d KBps, Packets Sent: %5d (%f pps), KB Sent: %d, UDP Overhead: %.2f %%.", n.KbpsSent, n.PacketsSent, float64(n.PacketsSent*1000/int64(now-n.StatsStartTime)), totalBytesSent/1024.0, udpOverhead))
	}
}

func (n *Netplay) OnMsg(msg *NetplayMsgType) {
	handled := false

	// filter out messages that don't match what we expect
	seq := msg.Hdr.SequenceNumber
	if msg.Hdr.Type != SyncRequest && msg.Hdr.Type != SyncReply {
		if msg.Hdr.Magic != n.RemoteMagicNumber {
			//LogMsg("recv rejecting", msg)
			return
		}

		// filter out out-of-order packets
		skipped := uint64(seq - n.NextRecvSeq)
		logrus.Info(fmt.Sprintf("Checking sequence number -> next - seq : %d - %d = %d", seq, n.NextRecvSeq, skipped))
		if skipped > MAX_SEQ_DISTANCE {
			logrus.Info(fmt.Sprintf("Dropping out of order packet (seq: %d, last seq:%d)", seq, n.NextRecvSeq))
			return
		}
	}

	n.NextRecvSeq = seq
	//LogMsg("recv", msg);

	switch msg.Hdr.Type {
	case SyncRequest:
		handled = n.OnSyncRequest(msg)
		break
	case SyncReply:
		handled = n.OnSyncReply(msg)
		break
	case Input:
		handled = n.OnInput(msg)
		break
	case QualityReport:
		handled = n.OnQualityReport(msg)
		break
	case QualityReply:
		handled = n.OnQualityReply(msg)
		break
	case KeepAlive:
		handled = n.OnKeepAlive(msg)
		break
	case InputAck:
		handled = n.OnInputAck(msg)
		break
	case GameState:
		handled = n.OnGameState(msg)
		break
	default:
		handled = false
	}

	if handled {
		n.LastRecvTime = platform.GetCurrentTimeMS()
		if n.DisconnectNotifySent && n.CurrentState == Running {
			var evt Event
			evt.Init(EventNetworkResumed)
			n.QueueEvent(&evt)
			n.DisconnectNotifySent = false
		}
	}
}

func (n *Netplay) Synchronize() {
	n.CurrentState = Syncing
	n.NetplayState.Sync.RoundTripsRemaining = NUM_SYNC_PACKETS
	n.SendSyncRequest()
}

func (n *Netplay) IsSynchronized() bool {
	return n.CurrentState == Running
}

func (n *Netplay) OnLoopPoll() bool {
	messages := make([]NetplayMsgType, len(n.ReceiveChannel))
	for i := 0; i < len(n.ReceiveChannel); i++ {
		messages[i] = <-n.ReceiveChannel
	}

	for i := 0; i < len(messages); i++ {
		n.OnMsg(&messages[i])
	}

	var now uint64 = platform.GetCurrentTimeMS()
	var nextInterval uint64

	n.PumpSendQueue()
	switch n.CurrentState {
	case Syncing:
		if n.NetplayState.Sync.RoundTripsRemaining == NUM_SYNC_PACKETS {
			nextInterval = SYNC_FIRST_RETRY_INTERVAL
		} else {
			nextInterval = SYNC_RETRY_INTERVAL
		}
		if n.LastSendTime > 0 && n.LastSendTime+nextInterval < now {
			logrus.Info(fmt.Sprintf("No luck syncing after %d ms... Re-queueing sync packet.", nextInterval))
			n.SendSyncRequest()
		}
		break

	case Running:
		// xxx: rig all this up with a timer wrapper
		if n.NetplayState.Running.LastInputPacketRecvTime <= 0 || n.NetplayState.Running.LastInputPacketRecvTime+RUNNING_RETRY_INTERVAL < now {
			logrus.Info(fmt.Sprintf("Haven't exchanged packets in a while (last received:%d  last sent:%d).  Resending.", n.LastReceivedInput.Frame, n.LastSentInput.Frame))
			n.SendPendingOutput()
			n.NetplayState.Running.LastInputPacketRecvTime = now
		}

		if n.NetplayState.Running.LastQualityReportTime <= 0 || n.NetplayState.Running.LastQualityReportTime+QUALITY_REPORT_INTERVAL < now {
			var msg *NetplayMsgType = new(NetplayMsgType)
			msg.Init(QualityReport)
			msg.QualityReport.Ping = int64(platform.GetCurrentTimeMS())
			msg.QualityReport.FrameAdvantage = n.LocalFrameAdvantage
			n.SendMsg(msg)
			n.NetplayState.Running.LastQualityReportTime = now
		}

		if n.NetplayState.Running.LastNetworkStatsInterval <= 0 || n.NetplayState.Running.LastNetworkStatsInterval+NETWORK_STATS_INTERVAL < now {
			n.UpdateNetworkStats()
			n.NetplayState.Running.LastNetworkStatsInterval = now
		}

		if n.LastSendTime > 0 && n.LastSendTime+KEEP_ALIVE_INTERVAL < now {
			logrus.Info("Sending keep alive packet")
			var msg *NetplayMsgType = new(NetplayMsgType)
			msg.Init(KeepAlive)
			n.SendMsg(msg)
		}

		if n.DisconnectTimeout > 0 && n.DisconnectNotifyStart > 0 && !n.DisconnectNotifySent && (n.LastRecvTime+uint64(n.DisconnectNotifyStart) < now) {
			logrus.Info(fmt.Sprintf("Endpoint has stopped receiving packets for %d ms. Sending notification.", n.DisconnectNotifyStart))
			var evt Event
			evt.Init(EventNetworkInterrupted)
			evt.DisconnectTimeout = n.DisconnectTimeout - n.DisconnectNotifyStart
			n.QueueEvent(&evt)
			n.DisconnectNotifySent = true
		}

		if n.DisconnectTimeout > 0 && (n.LastRecvTime+uint64(n.DisconnectTimeout) < now) {
			if !n.DisconnectEventSent {
				logrus.Info(fmt.Sprintf("Endpoint has stopped receiving packets for %d ms. Disconnecting.", n.DisconnectTimeout))
				var evt Event
				evt.Init(EventDisconnected)
				n.QueueEvent(&evt)
				n.DisconnectEventSent = true
			}
		}
		break

	case Disconnected:
		if n.ShutDownTimeout < int64(now) {
			logrus.Info("Shutting down udp connection.")
			n.ShutDownTimeout = 0
		}

	}
	return true
}
