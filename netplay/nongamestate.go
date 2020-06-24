package netplay

import (
	"github.com/libretro/ludo/ggpo/ggponet"
)

const MAX_PLAYERS = 64

type PlayerConnectState int64

const (
	Connecting    PlayerConnectState = 0
	Synchronizing PlayerConnectState = 1
	Running       PlayerConnectState = 2
	Disconnected  PlayerConnectState = 3
	Disconnecting PlayerConnectState = 4
)

type PlayerConnectionInfo struct {
	Type              ggponet.GGPOPlayerType
	Handle            ggponet.GGPOPlayerHandle
	State             PlayerConnectState
	ConnectProgress   int64
	DisconnectTimeout int64
	DisconnectStart   int64
}

type ChecksumInfo struct {
	FrameNumber int64
	Checksum    int64
}

type NonGameState struct {
	LocalPlayerHandle ggponet.GGPOPlayerHandle
	Players           [MAX_PLAYERS]PlayerConnectionInfo
	NumPlayers        int64
	Now               ChecksumInfo
	Periodic          ChecksumInfo
}

func (n *NonGameState) SetConnectState(handle ggponet.GGPOPlayerHandle, state PlayerConnectState) {
	for i := 0; i < int(n.NumPlayers); i++ {
		if n.Players[i].Handle == handle {
			n.Players[i].ConnectProgress = 0
			n.Players[i].State = state
			break
		}
	}
}

func (n *NonGameState) SetDisconnectTimeout(handle ggponet.GGPOPlayerHandle, when int64, timeout int64) {
	for i := 0; i < int(n.NumPlayers); i++ {
		if n.Players[i].Handle == handle {
			n.Players[i].DisconnectStart = when
			n.Players[i].DisconnectTimeout = timeout
			n.Players[i].State = Disconnecting
			break
		}
	}
}

func (n *NonGameState) SetAllConnectState(state PlayerConnectState) {
	for i := 0; i < int(n.NumPlayers); i++ {
		n.Players[i].State = state
	}
}

func (n *NonGameState) UpdateConnectProgress(handle ggponet.GGPOPlayerHandle, progress int64) {
	for i := 0; i < int(n.NumPlayers); i++ {
		if n.Players[i].Handle == handle {
			n.Players[i].ConnectProgress = progress
			break
		}
	}
}
