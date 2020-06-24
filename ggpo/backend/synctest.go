package backend

import (
	"fmt"

	"github.com/libretro/ludo/ggpo/ggponet"
	"github.com/libretro/ludo/ggpo/lib"
	"github.com/sirupsen/logrus"
)

type SyncTestBackend struct {
	Callbacks     ggponet.GGPOSessionCallbacks
	NumPlayers    int64
	CheckDistance int64
	LastVerified  int64
	RollingBack   bool
	Running       bool
	Game          string
	CurrentInput  lib.GameInput
	LastInput     lib.GameInput
	SavedFrame    lib.RingBuffer
	Sync          lib.Sync
}

type SavedInfo struct {
	Frame    int64
	Checksum int64
	Buf      []byte
	Cbuf     int64
	Input    lib.GameInput
}

func (s *SyncTestBackend) Init(cb *ggponet.GGPOSessionCallbacks, gamename string, frames int64, numPlayers int64) {
	s.Callbacks = *cb
	s.NumPlayers = numPlayers
	s.CheckDistance = frames
	s.LastVerified = 0
	s.RollingBack = false
	s.Running = false
	s.Game = gamename
	s.CurrentInput.Erase()
	s.SavedFrame.Init(32)

	var config lib.Config
	config.Callbacks = s.Callbacks
	config.NumPredictionFrames = lib.MAX_PREDICTION_FRAMES
	s.Sync.Init(config, s.Sync.LocalConnectStatus)

	s.Callbacks.BeginGame(s.Game)
}

func (s *SyncTestBackend) DoPoll(timeout int64) ggponet.GGPOErrorCode {
	if !s.Running {
		var info ggponet.GGPOEvent

		info.Code = ggponet.GGPO_EVENTCODE_RUNNING
		s.Callbacks.OnEvent(&info)
		s.Running = true
	}
	return ggponet.GGPO_OK
}

func (s *SyncTestBackend) AddPlayer(player *ggponet.GGPOPlayer, handle *ggponet.GGPOPlayerHandle) ggponet.GGPOErrorCode {
	if player.PlayerNum < 1 || player.PlayerNum > s.NumPlayers {
		return ggponet.GGPO_ERRORCODE_PLAYER_OUT_OF_RANGE
	}
	*handle = (ggponet.GGPOPlayerHandle)(player.PlayerNum - 1)
	return ggponet.GGPO_OK
}

func (s *SyncTestBackend) AddLocalInput(player ggponet.GGPOPlayerHandle, values []byte, size int64) ggponet.GGPOErrorCode {
	if !s.Running {
		return ggponet.GGPO_ERRORCODE_NOT_SYNCHRONIZED
	}

	var index int64 = int64(player)
	for i := 0; i < int(size); i++ {
		s.CurrentInput.Bits[(index * size)] |= values[i]
	}
	return ggponet.GGPO_OK
}

func (s *SyncTestBackend) SyncInput(values []byte, size int64, disconnectFlags *int64) {
	if s.RollingBack {
		var saved *SavedInfo = (*s.SavedFrame.Front()).(*SavedInfo)
		s.LastInput = saved.Input
	} else {
		if s.Sync.FrameCount == 0 {
			s.Sync.SaveCurrentFrame()
		}
		s.LastInput = s.CurrentInput
	}
	s.LastInput.Bits = values
	if *disconnectFlags == int64(1) {
		*disconnectFlags = 0
	}
}

func (s *SyncTestBackend) IncrementFrame() ggponet.GGPOErrorCode {
	s.Sync.IncrementFrame()
	s.CurrentInput.Erase()

	logrus.Info(fmt.Sprintf("End of frame(%d)...", s.Sync.FrameCount))

	if s.RollingBack {
		return ggponet.GGPO_OK
	}

	frame := s.Sync.FrameCount
	// Hold onto the current frame in our queue of saved states.  We'll need
	// the checksum later to verify that our replay of the same frame got the
	// same results.
	var info *SavedInfo
	info.Frame = frame
	info.Input = s.LastInput
	info.Cbuf = s.Sync.GetLastSavedFrame().Cbuf
	info.Buf = s.Sync.GetLastSavedFrame().Buf
	info.Checksum = s.Sync.GetLastSavedFrame().Checksum
	var t lib.T = &info
	s.SavedFrame.Push(&t)

	if frame-s.LastVerified == s.CheckDistance {
		// We've gone far enough ahead and should now start replaying frames.
		// Load the last verified frame and set the rollback flag to true.
		s.Sync.LoadFrame(s.LastVerified)
		s.RollingBack = true
		for !s.SavedFrame.Empty() {
			s.Callbacks.AdvanceFrame(0)

			// Verify that the checksumn of this frame is the same as the one in our list
			info = (*s.SavedFrame.Front()).(*SavedInfo)
			s.SavedFrame.Pop()

			if info.Frame != s.Sync.FrameCount {
				logrus.Info(fmt.Sprintf("Frame number %d does not match saved frame number %d", info.Frame, frame))
			}
			checksum := s.Sync.GetLastSavedFrame().Checksum
			if info.Checksum != checksum {
				logrus.Info(fmt.Sprintf("FrameCount : %d , LastSavedFrame.buf : %x , LastSavedFrame.cbuf : %d",
					s.Sync.FrameCount, s.Sync.GetLastSavedFrame().Buf, s.Sync.GetLastSavedFrame().Cbuf))
				logrus.Info(fmt.Sprintf("Checksum for frame %d does not match saved (%d != %d)", frame, checksum, info.Checksum))
			}
			println()

		}
		s.LastVerified = frame
		s.RollingBack = false
	}
	return ggponet.GGPO_OK
}
