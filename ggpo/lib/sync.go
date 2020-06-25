package lib

import (
	"fmt"

	"github.com/libretro/ludo/ggpo/ggponet"
	"github.com/sirupsen/logrus"
)

const MAX_PREDICTION_FRAMES = 8

type Sync struct {
	Rollingback         bool
	LastConfirmedFrame  int64
	FrameCount          int64
	MaxPredictionFrames int64
	SavedState          SavedState
	InputQueues         []InputQueue
	Config              Config
	Callbacks           ggponet.GGPOSessionCallbacks
	EventQueue          RingBuffer
	LocalConnectStatus  []ggponet.ConnectStatus
}

func (s *Sync) Init(config Config, ConnectStatus []ggponet.ConnectStatus) {
	s.Config = config
	s.Callbacks = config.Callbacks
	s.FrameCount = 0
	s.Rollingback = false
	s.LocalConnectStatus = ConnectStatus

	s.MaxPredictionFrames = config.NumPredictionFrames

	s.EventQueue.Init(32)

	s.CreateQueues(config)
}

type SavedFrame struct {
	Buf      []byte
	Cbuf     int64
	Frame    int64
	Checksum int64
}

func (s *SavedFrame) Init() {
	s.Buf = nil
	s.Cbuf = 0
	s.Frame = -1
	s.Checksum = 0
}

type SavedState struct {
	Frames [MAX_PREDICTION_FRAMES + 2]SavedFrame
	Head   int64
}

type Config struct {
	Callbacks           ggponet.GGPOSessionCallbacks
	NumPredictionFrames int64
	NumPlayers          int64
	InputSize           int64
}

type Event struct {
	ConfirmedInput int64
	Input          GameInput
}

func (s *Sync) SetLastConfirmedFrame(frame int64) {
	s.LastConfirmedFrame = frame
	if s.LastConfirmedFrame > 0 {
		for i := 0; i < int(s.Config.NumPlayers); i++ {
			s.InputQueues[i].DiscardConfirmedFrames(frame - 1)
		}
	}
}

func (s *Sync) AddLocalInput(queue int64, input *GameInput) bool {
	framesBehind := s.FrameCount - s.LastConfirmedFrame
	if s.FrameCount >= s.MaxPredictionFrames && framesBehind >= s.MaxPredictionFrames {
		logrus.Info("Rejecting input from emulator: reached prediction barrier.")
		return false
	}

	if s.FrameCount == 0 {
		s.SaveCurrentFrame()
	}

	logrus.Info(fmt.Sprintf("Sending undelayed local frame %d to queue %d", s.FrameCount, queue))
	input.Frame = s.FrameCount
	s.InputQueues[queue].AddInput(input)

	return true
}

func (s *Sync) AddRemoteInput(queue int64, input *GameInput) {
	s.InputQueues[queue].AddInput(input)
}

func (s *Sync) GetConfirmedInputs(values []byte, size int64, frame int64) int64 {
	var disconnectFlags int64 = 0
	output := values

	if size < s.Config.NumPlayers*s.Config.InputSize {
		logrus.Panic("Assert error size")
	}

	for i := 0; i < int(s.Config.NumPlayers); i++ {
		var input GameInput
		if s.LocalConnectStatus[i].Disconnected && frame > s.LocalConnectStatus[i].LastFrame {
			disconnectFlags |= (1 << i)
			input.Erase()
		} else {
			s.InputQueues[i].GetConfirmedInput(frame, &input)
		}
		for k := 0; k < i*int(s.Config.InputSize); k += int(s.Config.InputSize) {
			for j := 0; j < int(s.Config.InputSize); j++ {
				output[k+j] = input.Bits[j]
			}
		}
	}
	return disconnectFlags
}

func (s *Sync) SynchronizeInputs(values []byte, size int64) int64 {
	var disconnectedFlags int64 = 0

	if size < s.Config.NumPlayers*s.Config.InputSize {
		logrus.Panic("Assert error size")
	}

	controllerSize := int(size / ggponet.GGPO_MAX_PLAYERS)

	output := make([]byte, size)
	for i := 0; i < int(s.Config.NumPlayers); i++ {
		var input GameInput
		if s.LocalConnectStatus[i].Disconnected && s.FrameCount > s.LocalConnectStatus[i].LastFrame {
			disconnectedFlags += 1 << i
			input.Bits = nil
		} else {
			s.InputQueues[i].GetInput(s.FrameCount, &input)
		}
		for j := 0; j < len(input.Bits); j++ {
			output[controllerSize*i+j] = input.Bits[j] //TODO: Maybe not concatenate but make has many slices in output that there are players
		}
	}
	for i := 0; i < len(output); i++ {
		if i >= len(values) {
			break
		}
		values[i] = output[i]
	}

	return disconnectedFlags
}

func (s *Sync) CheckSimulation() {
	var seekTo int64
	if !s.CheckSimulationConsistency(&seekTo) {
		s.AdjustSimulation(seekTo)
	}
}

func (s *Sync) IncrementFrame() {
	s.FrameCount++
	s.SaveCurrentFrame()
}

func (s *Sync) AdjustSimulation(seekTo int64) {
	framecount := s.FrameCount
	count := s.FrameCount - seekTo

	logrus.Info("Catching up")
	s.Rollingback = true

	// Flush our input queue and load the last frame
	s.LoadFrame(seekTo)
	if s.FrameCount != seekTo {
		logrus.Panic("Assert error seekTo")
	}

	//Advance frame by frame (stuffign notifications back to master)
	s.ResetPrediction(s.FrameCount)
	for i := 0; i < int(count); i++ {
		s.Callbacks.AdvanceFrame(0)
	}

	if s.FrameCount != framecount {
		logrus.Panic("Assert error framecount")
	}

	s.Rollingback = false
}

func (s *Sync) LoadFrame(frame int64) {

	// Find the frame in question
	if frame == s.FrameCount {
		logrus.Info("Skipping NOP")
		return
	}

	// Move the head pointer back and load it up
	s.SavedState.Head = s.FindSavedFrameIndex(frame)
	var state *SavedFrame = &s.SavedState.Frames[s.SavedState.Head]

	logrus.Info(fmt.Sprintf("Loading frame info %d (size: %d  checksum: %08x).", state.Frame, state.Cbuf, state.Checksum))

	s.Callbacks.LoadGameState(state.Buf, state.Cbuf)

	// Reset framecount and the head of the state ring-buffer to point in
	// advance of the current frame (as if we had just finished executing it).
	s.FrameCount = state.Frame
	s.SavedState.Head = (s.SavedState.Head + 1) % int64(len(s.SavedState.Frames))
}

// SaveCurrentFrame write everything into the head, then advance the head pointer
func (s *Sync) SaveCurrentFrame() {
	/*
	 * See StateCompress for the real save feature implemented by FinalBurn.
	 * Write everything into the head, then advance the head pointer.
	 */
	var state *SavedFrame = &s.SavedState.Frames[s.SavedState.Head]
	if state.Buf != nil {
		state.Buf = nil
	}
	state.Frame = s.FrameCount
	s.Callbacks.SaveGameState(state.Buf, &state.Cbuf, &state.Checksum, state.Frame)

	logrus.Info(fmt.Sprintf("Saved frame info %d (size: %d  checksum: %08x).",
		state.Frame, state.Cbuf, state.Checksum))

	s.SavedState.Head = (s.SavedState.Head + 1) % int64(len(s.SavedState.Frames))
}

func (s *Sync) GetLastSavedFrame() SavedFrame {
	i := s.SavedState.Head - 1
	if i < 0 {
		i = int64(len(s.SavedState.Frames)) - 1
	}
	return s.SavedState.Frames[i]
}

func (s *Sync) FindSavedFrameIndex(frame int64) int64 {
	var i int64
	var count int64 = int64(len(s.SavedState.Frames))
	for i = 0; i < count; i++ {
		if s.SavedState.Frames[i].Frame == frame {
			break
		}
	}
	if i == count {
		logrus.Panic("Assert Error FindSavedFrameIndex i == count")
	}
	return i
}

func (s *Sync) CreateQueues(config Config) bool {
	s.InputQueues = make([]InputQueue, config.NumPlayers)

	for i := 0; i < int(s.Config.NumPlayers); i++ {
		s.InputQueues[i].Init(int64(i), s.Config.InputSize)
	}
	return true
}

func (s *Sync) CheckSimulationConsistency(seekTo *int64) bool {
	firstIncorrect := NULL_FRAME
	for i := 0; i < int(s.Config.NumPlayers); i++ {
		incorrect := s.InputQueues[i].FirstIncorrectFrame
		logrus.Info(fmt.Sprintf("Considering incorrect frame %d reported by queue %d.", incorrect, i))

		if incorrect != NULL_FRAME && (firstIncorrect == NULL_FRAME || incorrect < int64(firstIncorrect)) {
			firstIncorrect = int(incorrect)
		}
	}

	if firstIncorrect == NULL_FRAME {
		logrus.Info("Prediction ok. Proceeding.")
		return true
	}
	*seekTo = int64(firstIncorrect)
	return false
}

func (s *Sync) SetFrameDelay(queue int64, delay int64) {
	s.InputQueues[queue].SetFrameDelay(delay)
}

func (s *Sync) ResetPrediction(frameNumber int64) {
	for i := 0; i < int(s.Config.NumPlayers); i++ {
		s.InputQueues[i].ResetPrediction(frameNumber)
	}
}

func (s *Sync) GetEvent(e *Event) bool {
	if s.EventQueue.Size != 0 {
		e = (*s.EventQueue.Front()).(*Event)
		s.EventQueue.Pop()
		return true
	}
	return false
}
