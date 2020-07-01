package backend

import (
	"bytes"
	"fmt"
	"net"

	"github.com/libretro/ludo/ggpo/ggponet"
	"github.com/libretro/ludo/ggpo/lib"
	"github.com/libretro/ludo/ggpo/network"
	"github.com/sirupsen/logrus"
)

const (
	RECOMMENDATION_INTERVAL         = 240
	DEFAULT_DISCONNECT_TIMEOUT      = 5000
	DEFAULT_DISCONNECT_NOTIFY_START = 750
)

type Peer2PeerBackend struct {
	Poll                  lib.Poll
	Spectators            []network.Netplay
	LocalConnectStatus    []ggponet.ConnectStatus
	Endpoints             []network.Netplay
	Players               []ggponet.GGPOPlayer
	Sync                  lib.Sync
	InputSize             int64
	NumPlayers            int64
	NumSpectators         int64
	NextSpectatorFrame    int64
	NextRecommendedSleep  int64
	DisconnectTimeout     int64
	DisconnectNotifyStart int64
	LocalPlayerIndex      int64
	Synchronizing         bool
	LocalPort             string
	Callbacks             ggponet.GGPOSessionCallbacks
	HostingConn           *net.UDPConn
}

func (p *Peer2PeerBackend) Init(cb ggponet.GGPOSessionCallbacks, gamename string, localPort string) {
	p.Callbacks = cb
	p.LocalPort = localPort
	p.Synchronizing = true
	p.NextRecommendedSleep = 0
	p.Poll.Init()
	var config lib.Config = lib.Config{}
	config.NumPlayers = p.NumPlayers
	config.InputSize = p.InputSize
	config.Callbacks = p.Callbacks
	config.NumPredictionFrames = lib.MAX_PREDICTION_FRAMES
	p.HostingConn = nil

	p.Players = make([]ggponet.GGPOPlayer, p.NumPlayers)
	p.Endpoints = make([]network.Netplay, p.NumPlayers)
	p.Spectators = make([]network.Netplay, p.NumSpectators)
	p.LocalConnectStatus = make([]ggponet.ConnectStatus, p.NumPlayers)
	p.Sync.Init(config, p.LocalConnectStatus)
	for i := 0; i < len(p.LocalConnectStatus); i++ {
		p.LocalConnectStatus[i].LastFrame = -1
	}

	p.Callbacks.BeginGame(gamename)
}

func (p *Peer2PeerBackend) AddRemotePlayer(player *ggponet.GGPOPlayer, localPort string, queue int64) {
	p.Synchronizing = true
	var c network.Callbacks = p
	p.Endpoints[queue].Init(*player, localPort, queue, p.LocalConnectStatus, &p.Poll, &c)
	p.HostingConn = p.Endpoints[queue].HostConnection(p.HostingConn)
	p.Endpoints[queue].SetDisconnectTimeout(p.DisconnectTimeout)
	p.Endpoints[queue].SetDisconnectNotifyStart(p.DisconnectNotifyStart)
	p.Endpoints[queue].Synchronize()
}

func (p *Peer2PeerBackend) AddLocalInput(player ggponet.GGPOPlayerHandle, values []byte, size int64) ggponet.GGPOErrorCode {
	var queue int64
	var input lib.GameInput
	var result ggponet.GGPOErrorCode

	if p.Sync.Rollingback {
		return ggponet.GGPO_ERRORCODE_IN_ROLLBACK
	}
	if p.Synchronizing {
		return ggponet.GGPO_ERRORCODE_NOT_SYNCHRONIZED
	}

	result = p.PlayerHandleToQueue(player, &queue)
	if !ggponet.GGPO_SUCCEEDED(result) {
		return result
	}

	input.SimpleInit(-1, values, size)

	// Feed the input for the current frame into the synchronzation layer.
	if !p.Sync.AddLocalInput(queue, &input) {
		return ggponet.GGPO_ERRORCODE_PREDICTION_THRESHOLD
	}

	if input.Frame != lib.NULL_FRAME {
		logrus.Info(fmt.Sprintf("setting local connect status for local queue %d to %d", queue, input.Frame))
		p.LocalConnectStatus[queue].LastFrame = input.Frame

		// Send the input to all the remote players.
		for i := 0; i < int(p.NumPlayers); i++ {
			if int64(i) != p.LocalPlayerIndex {
				p.Endpoints[i].SendInput(&input)
			}
		}
	}

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) SyncInput(values []byte, size int64, disconnectFlags *int64) ggponet.GGPOErrorCode {
	var flags int64

	// Wait until we've started to return inputs
	if p.Synchronizing {
		return ggponet.GGPO_ERRORCODE_NOT_SYNCHRONIZED
	}

	diff := make([]byte, size)
	logrus.Info("Values before : ", values) //TODO: Remove logrus here
	flags = p.Sync.SynchronizeInputs(values, size)
	if bytes.Compare(diff, values) != 0 { //TODO: Remove logrus here
		logrus.Info("Values after : ", values)
	}
	if *disconnectFlags != 0 {
		*disconnectFlags = flags
	}

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) DoPoll() ggponet.GGPOErrorCode {
	if !p.Sync.Rollingback {
		p.Poll.Pump()

		p.PollNetplayEvents()

		if !p.Synchronizing {
			p.Sync.CheckSimulation()

			// notify all of our endpoints of their local frame number for their
			// next connection quality report
			currentFrame := p.Sync.FrameCount
			for i := 0; i < int(p.NumPlayers); i++ {
				p.Endpoints[i].SetLocalFrameNumber(currentFrame)
			}

			var totalMinConfirmed int64
			if p.NumPlayers <= 2 {
				totalMinConfirmed = p.Poll2Players(currentFrame)
			} else {
				totalMinConfirmed = p.PollNPlayers(currentFrame)
			}

			logrus.Info(fmt.Sprintf("last confirmed frame in p2p backend is %d.", totalMinConfirmed))
			if totalMinConfirmed >= 0 {
				if p.NumSpectators > 0 {
					for p.NextSpectatorFrame <= totalMinConfirmed {
						logrus.Info(fmt.Sprintf("pushing frame %d to spectators.", p.NextSpectatorFrame))

						var input lib.GameInput
						input.Frame = p.NextSpectatorFrame
						input.Size = p.InputSize * p.NumPlayers
						p.Sync.GetConfirmedInputs(input.Bits, p.InputSize*p.NumPlayers, p.NextSpectatorFrame)
						for i := 0; i < int(p.NumSpectators); i++ {
							p.Spectators[i].SendInput(&input)
						}
						p.NextSpectatorFrame++
					}
				}
				logrus.Info(fmt.Sprintf("setting confirmed frame in sync to %d.", totalMinConfirmed))
				p.Sync.SetLastConfirmedFrame(totalMinConfirmed)
			}

			// send timesync notifications if now is the proper time
			if currentFrame > p.NextRecommendedSleep {
				interval := int64(0)
				for i := 0; i < int(p.NumPlayers); i++ {
					interval = lib.MAX(interval, p.Endpoints[i].RecommendFrameDelay())
				}

				if interval > 0 {
					var info ggponet.GGPOEvent
					info.Code = ggponet.GGPO_EVENTCODE_TIMESYNC
					info.TimeSync.FramesAhead = interval
					p.Callbacks.OnEvent(&info)
					p.NextRecommendedSleep = currentFrame + RECOMMENDATION_INTERVAL
				}
			}
		}
	}
	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) Poll2Players(currentFrame int64) int64 {
	totalMinConfirmed := int64(lib.MAX_INT)
	for i := 0; i < int(p.NumPlayers); i++ {
		if int64(i) != p.LocalPlayerIndex {
			var ignore int64
			queueConnected := p.Endpoints[i].GetPeerConnectStatus(int64(i), &ignore)

			if !p.LocalConnectStatus[i].Disconnected {
				totalMinConfirmed = lib.MIN(p.LocalConnectStatus[i].LastFrame, totalMinConfirmed)
			}

			logrus.Info(fmt.Sprintf("local endp: connected = %t, last_received = %d, totalMinConfirmed = %d.",
				!p.LocalConnectStatus[i].Disconnected, p.LocalConnectStatus[i].LastFrame, totalMinConfirmed))

			if !queueConnected && !p.LocalConnectStatus[i].Disconnected {
				logrus.Info(fmt.Sprintf("disconnecting i %d by remote request.", i))
				p.DisconnectPlayerQueue(int64(i), totalMinConfirmed)
			}
			logrus.Info(fmt.Sprintf("totalMinConfirmed = %d.", totalMinConfirmed))
		}
	}
	return totalMinConfirmed
}

func (p *Peer2PeerBackend) PollNPlayers(currentFrame int64) int64 {
	var lastReceived int64
	// discard confirmed frames as appropriate
	totalMinConfirmed := int64(lib.MAX_INT)
	for queue := 0; queue < int(p.NumPlayers); queue++ {
		if int64(queue) != p.LocalPlayerIndex {
			queueConnected := true
			queueMinConfirmed := int64(lib.MAX_INT)
			logrus.Info(fmt.Sprintf("considering queue %d.", queue))
			for i := 0; i < int(p.NumPlayers); i++ {
				if int64(i) != p.LocalPlayerIndex {
					// we're going to do a lot of logic here in consideration of endpoint i.
					// keep accumulating the minimum confirmed point for all n*n packets and
					// throw away the rest.
					connected := p.Endpoints[i].GetPeerConnectStatus(int64(queue), &lastReceived)

					queueConnected = queueConnected && connected
					queueMinConfirmed = int64(lib.MIN(lastReceived, queueMinConfirmed))
					logrus.Info(fmt.Sprintf("endpoint %d: connected = %t, last_received = %d, queueMinConfirmed = %d.",
						i, connected, lastReceived, queueMinConfirmed))
				}
			}
			// merge in our local status only if we're still connected!
			if !p.LocalConnectStatus[queue].Disconnected {
				queueMinConfirmed = lib.MIN(p.LocalConnectStatus[queue].LastFrame, queueMinConfirmed)
			}

			logrus.Info(fmt.Sprintf("local endp: connected = %t, last_received = %d, queueMinConfirmed = %d.",
				!p.LocalConnectStatus[queue].Disconnected, p.LocalConnectStatus[queue].LastFrame, queueMinConfirmed))

			if queueConnected {
				totalMinConfirmed = lib.MIN(queueMinConfirmed, totalMinConfirmed)
			} else {
				// check to see if this disconnect notification is further back than we've been before.  If
				// so, we need to re-adjust.  This can happen when we detect our own disconnect at frame n
				// and later receive a disconnect notification for frame n-1.
				if !p.LocalConnectStatus[queue].Disconnected || p.LocalConnectStatus[queue].LastFrame > queueMinConfirmed {
					logrus.Info(fmt.Sprintf("disconnecting queue %d by remote request.", queue))
					p.DisconnectPlayerQueue(int64(queue), queueMinConfirmed)
				}
			}
			logrus.Info(fmt.Sprintf("totalMinConfirmed = %d.", totalMinConfirmed))
		}
	}
	return totalMinConfirmed
}

func (p *Peer2PeerBackend) PollNetplayEvents() {
	var evt *network.Event = new(network.Event)
	evt.Init(network.EventUnknown)
	for i := 0; i < int(p.NumPlayers); i++ {
		if i != int(p.LocalPlayerIndex) {
			for p.Endpoints[i].GetEvent(evt) {
				p.OnNetplayPeerEvent(evt, int64(i))
			}
		}
	}
	for i := 0; i < int(p.NumSpectators); i++ {
		for p.Spectators[i].GetEvent(evt) {
			p.OnNetplaySpectatorEvent(evt, int64(i))
		}
	}
}

func (p *Peer2PeerBackend) OnNetplayPeerEvent(evt *network.Event, queue int64) {
	p.OnNetplayEvent(evt, p.QueueToPlayerHandle(queue))
	switch evt.Type {
	case network.EventInput:
		if !p.LocalConnectStatus[queue].Disconnected {
			currentRemoteFrame := p.LocalConnectStatus[queue].LastFrame
			newRemoteFrame := evt.Input.Frame
			if currentRemoteFrame != -1 && newRemoteFrame != (currentRemoteFrame+1) {
				logrus.Panic("Assert error on remote frame in netplay peer event")
			}

			p.Sync.AddRemoteInput(queue, &evt.Input)
			// Notify the other endpoints which frame we received from a peer
			logrus.Info(fmt.Sprintf("setting remote connect status for queue %d to %d", queue, evt.Input.Frame))
			p.LocalConnectStatus[queue].LastFrame = evt.Input.Frame
		}
		break
	case network.EventGameState:
		//TODO: Rollback if checksum doesn't match
		// if evt.SavedFrame.Checksum != p.Sync.GetLastSavedFrame().Checksum {
		// 	p.Sync.AdjustSimulation(p.Sync.FrameCount - 1)
		// }
		break
	case network.EventDisconnected:
		p.DisconnectPlayer(p.QueueToPlayerHandle(queue))
		break
	}
}

func (p *Peer2PeerBackend) OnNetplaySpectatorEvent(evt *network.Event, queue int64) {
	//TODO: Spectators
}

func (p *Peer2PeerBackend) OnNetplayEvent(evt *network.Event, handle ggponet.GGPOPlayerHandle) {
	var info ggponet.GGPOEvent

	switch evt.Type {
	case network.EventConnected:
		info.Code = ggponet.GGPO_EVENTCODE_CONNECTED_TO_PEER
		info.Connected.Player = handle
		p.Callbacks.OnEvent(&info)
		break

	case network.EventSynchronizing:
		info.Code = ggponet.GGPO_EVENTCODE_SYNCHRONIZING_WITH_PEER
		info.Synchronizing.Player = handle
		info.Synchronizing.Count = evt.Synchronizing.Count
		info.Synchronizing.Total = evt.Synchronizing.Total
		p.Callbacks.OnEvent(&info)
		break

	case network.EventSynchronized:
		info.Code = ggponet.GGPO_EVENTCODE_SYNCHRONIZED_WITH_PEER
		info.Synchronized.Player = handle
		p.Callbacks.OnEvent(&info)

		p.CheckInitialSync()
		break

	case network.EventNetworkInterrupted:
		info.Code = ggponet.GGPO_EVENTCODE_CONNECTION_INTERRUPTED
		info.ConnectionInterrupted.Player = handle
		info.ConnectionInterrupted.DisconnectTimeout = evt.DisconnectTimeout
		p.Callbacks.OnEvent(&info)
		break

	case network.EventNetworkResumed:
		info.Code = ggponet.GGPO_EVENTCODE_CONNECTION_RESUMED
		info.ConnectionResumed.Player = handle
		p.Callbacks.OnEvent(&info)
		break
	}
}

func (p *Peer2PeerBackend) OnMsg(recvAddr *net.UDPAddr, msg *network.NetplayMsgType) {
	for i := 0; i < int(p.NumPlayers); i++ {
		if p.LocalPlayerIndex != int64(i) && recvAddr != nil && p.Endpoints[i].HandlesMsg(recvAddr) {
			p.Endpoints[i].OnMsg(msg)
			return
		}
	}
	//TODO: Spectators
}

func (p *Peer2PeerBackend) AddPlayer(player *ggponet.GGPOPlayer, handle *ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	if player.Type == ggponet.GGPO_PLAYERTYPE_SPECTATOR {
		return p.AddSpectator(player.IPAddress, player.Port)
	}

	queue := player.PlayerNum - 1
	p.Players[queue] = *player
	if player.PlayerNum < 1 || player.PlayerNum > p.NumPlayers {
		return ggponet.GGPO_ERRORCODE_PLAYER_OUT_OF_RANGE
	}
	*handle = p.QueueToPlayerHandle(queue)

	if player.Type == ggponet.GGPO_PLAYERTYPE_REMOTE {
		p.AddRemotePlayer(player, p.LocalPort, queue)
	} else {
		p.LocalPlayerIndex = queue
	}

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) IncrementFrame() ggponet.GGPOErrorCode {
	logrus.Info(fmt.Sprintf("End of frame (%d)...", p.Sync.FrameCount))

	//Send game state to remote players
	for i := 0; i < int(p.NumPlayers); i++ {
		if int64(i) != p.LocalPlayerIndex {
			p.Endpoints[i].SendGameState(p.Sync.GetLastSavedFrame())
		}
	}
	p.Sync.IncrementFrame()
	p.DoPoll()
	p.PollSyncEvents()

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) PollSyncEvents() {
	var e *lib.Event
	for p.Sync.GetEvent(e) {
		p.OnSyncEvent(e)
	}
}

func (p *Peer2PeerBackend) OnSyncEvent(evt *lib.Event) {}

func (p *Peer2PeerBackend) DisconnectPlayer(player ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	var queue int64
	var result ggponet.GGPOErrorCode

	result = p.PlayerHandleToQueue(player, &queue)
	if !ggponet.GGPO_SUCCEEDED(result) {
		return result
	}

	if p.LocalConnectStatus[queue].Disconnected {
		return ggponet.GGPO_ERRORCODE_PLAYER_DISCONNECTED
	}

	currentFrame := p.Sync.FrameCount
	logrus.Info(fmt.Sprintf("Disconnecting local player %d at frame %d by user request.", queue, p.LocalConnectStatus[queue].LastFrame))
	var i int64 = 0
	for ; i < p.NumPlayers; i++ {
		p.DisconnectPlayerQueue(i, currentFrame)
	}

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) DisconnectPlayerQueue(queue int64, syncto int64) {
	var info ggponet.GGPOEvent
	framecount := p.Sync.FrameCount

	p.Endpoints[queue].Disconnect()

	p.LocalConnectStatus[queue].Disconnected = true
	p.LocalConnectStatus[queue].LastFrame = syncto

	if syncto < framecount {
		p.Sync.AdjustSimulation(syncto)
	}

	info.Code = ggponet.GGPO_EVENTCODE_DISCONNECTED_FROM_PEER
	info.Disconnected.Player = p.QueueToPlayerHandle(queue)
	p.Callbacks.OnEvent(&info)

	p.CheckInitialSync()
}

func (p *Peer2PeerBackend) CheckInitialSync() {
	if p.Synchronizing {
		// Check to see if everyone is now synchronized.  If so,
		// go ahead and tell the client that we're ok to accept input.
		for i := 0; i < int(p.NumPlayers); i++ {
			if !p.LocalConnectStatus[i].Disconnected && !p.Endpoints[i].IsSynchronized() && int64(i) != p.LocalPlayerIndex {
				return
			}
		}
		/*for i := 0; i < int(p.NumSpectators); i++ {
			if !p.Spectators[i].IsSynchronized() {
				logrus.Info("CheckInitialSync 2eme boucle return")
				return
			}
		}*/

		var info ggponet.GGPOEvent
		info.Code = ggponet.GGPO_EVENTCODE_RUNNING
		p.Callbacks.OnEvent(&info)
		p.Synchronizing = false
	}
}

func (p *Peer2PeerBackend) QueueToPlayerHandle(queue int64) ggponet.GGPOPlayerHandle {
	return (ggponet.GGPOPlayerHandle)(queue + 1)
}

func (p *Peer2PeerBackend) GetNetworkStats(stats *ggponet.GGPONetworkStats, player ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	var queue int64
	var result ggponet.GGPOErrorCode

	result = p.PlayerHandleToQueue(player, &queue)
	if !ggponet.GGPO_SUCCEEDED(result) {
		return result
	}

	p.Endpoints[queue].GetNetworkStats(stats)

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) SetFrameDelay(player ggponet.GGPOPlayerHandle, delay int64) ggponet.GGPOErrorCode {
	var queue int64
	var result ggponet.GGPOErrorCode

	result = p.PlayerHandleToQueue(player, &queue)
	if !ggponet.GGPO_SUCCEEDED(result) {
		return result
	}
	p.Sync.SetFrameDelay(queue, delay)

	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) PlayerHandleToQueue(player ggponet.GGPOPlayerHandle, queue *int64) ggponet.GGPOErrorCode {
	offset := ((int64)(player) - 1)
	if offset < 0 || offset >= p.NumPlayers {
		return ggponet.GGPO_ERRORCODE_INVALID_PLAYER_HANDLE
	}
	*queue = offset
	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) AddSpectator(ip string, port uint64) ggponet.GGPOErrorCode {
	//TODO: Spectators
	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) SetDisconnectTimeout(timeout int64) ggponet.GGPOErrorCode {
	p.DisconnectTimeout = timeout
	for i := 0; i < int(p.NumPlayers); i++ {
		if p.Endpoints[i].IsInitialized {
			p.Endpoints[i].SetDisconnectTimeout(p.DisconnectTimeout)
		}
	}
	return ggponet.GGPO_OK
}

func (p *Peer2PeerBackend) SetDisconnectNotifyStart(timeout int64) ggponet.GGPOErrorCode {
	p.DisconnectNotifyStart = timeout
	for i := 0; i < int(p.NumPlayers); i++ {
		if p.Endpoints[i].IsInitialized {
			p.Endpoints[i].SetDisconnectNotifyStart(p.DisconnectNotifyStart)
		}
	}
	return ggponet.GGPO_OK
}
