package ggpo

import (
	"github.com/libretro/ludo/ggpo/backend"
	"github.com/libretro/ludo/ggpo/ggponet"
)

// StartSession begins our game session
func StartSession(session **ggponet.GGPOSession, cb ggponet.GGPOSessionCallbacks, game string, numPlayers int64, inputSize int64, localPort string) ggponet.GGPOErrorCode {
	var p2p backend.Peer2PeerBackend = backend.Peer2PeerBackend{NumPlayers: numPlayers, InputSize: inputSize}
	p2p.Init(cb, game, localPort)
	var s ggponet.GGPOSession = &p2p
	*session = &s
	return ggponet.GGPO_OK
}

// AddPlayer allows to add player in our game session
func AddPlayer(ggpo *ggponet.GGPOSession, player *ggponet.GGPOPlayer, handle *ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).AddPlayer(player, handle)
}

// SetFrameDelay is used to set frame delay to local inputs
func SetFrameDelay(ggpo *ggponet.GGPOSession, player ggponet.GGPOPlayerHandle, frameDelay int64) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).SetFrameDelay(player, frameDelay)
}

// AddLocalInput is used to add a local input before fetching the inputs for the remote players
func AddLocalInput(ggpo *ggponet.GGPOSession, player ggponet.GGPOPlayerHandle, values []byte, size int64) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).AddLocalInput(player, values, size)
}

// Idle is used to define the time we allow ggpo to spent receive packets from other players during 1 frame
func Idle(ggpo *ggponet.GGPOSession) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).DoPoll()
}

func SynchronizeInput(ggpo *ggponet.GGPOSession, values []byte, size int64, disconnectFlags *int64) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).SyncInput(values, size, disconnectFlags)
}

func DisconnectPlayer(ggpo *ggponet.GGPOSession, player ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).DisconnectPlayer(player)
}

func AdvanceFrame(ggpo *ggponet.GGPOSession) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).IncrementFrame()
}

func GetNetworkStats(ggpo *ggponet.GGPOSession, player ggponet.GGPOPlayerHandle, stats *ggponet.GGPONetworkStats) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).GetNetworkStats(stats, player)
}

func SetDisconnectTimeout(ggpo *ggponet.GGPOSession, timeout int64) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).SetDisconnectTimeout(timeout)
}

func SetDisconnectNotifyStart(ggpo *ggponet.GGPOSession, timeout int64) ggponet.GGPOErrorCode {
	if ggpo == nil {
		return ggponet.GGPO_ERRORCODE_INVALID_SESSION
	}
	return (*ggpo).SetDisconnectNotifyStart(timeout)
}

/* TODOs :
- Check other players game state checksum
- Rollback if other players checksum is different
- Check prediction and synctest
- Refractor
*/
