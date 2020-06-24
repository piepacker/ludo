package lib

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

const (
	FRAME_WINDOW_SIZE   = 40
	MIN_UNIQUE_FRAMES   = 10
	MIN_FRAME_ADVANTAGE = 3
	MAX_FRAME_ADVANTAGE = 9
)

type TimeSync struct {
	Local          [FRAME_WINDOW_SIZE]int64
	Remote         [FRAME_WINDOW_SIZE]int64
	LastInputs     [MIN_UNIQUE_FRAMES]GameInput
	NextPrediction int64
	Count          int64
}

func (t *TimeSync) Init() {
	t.NextPrediction = FRAME_WINDOW_SIZE * 3
	t.Count = 0
}

func (t *TimeSync) AdvanceFrame(input *GameInput, advantage int64, radvantage int64) {
	// Remember the last frame and frame advantage
	t.LastInputs[input.Frame%MIN_UNIQUE_FRAMES] = *input
	t.Local[input.Frame%FRAME_WINDOW_SIZE] = advantage
	t.Remote[input.Frame%FRAME_WINDOW_SIZE] = radvantage
}

func (t *TimeSync) RecommendFrameWaitDuration(requireIdleInput bool) int64 {
	// Average our local and remote frame advantages
	sum := int64(0)
	var advantage, radvantage float64
	for i := 0; i < len(t.Local); i++ {
		sum += t.Local[i]
	}
	advantage = float64(sum) / float64(len(t.Local))

	sum = 0
	for i := 0; i < len(t.Remote); i++ {
		sum += t.Remote[i]
	}
	radvantage = float64(sum) / float64(len(t.Remote))

	t.Count++

	// See if someone should take action.  The person furthest ahead
	// needs to slow down so the other user can catch up.
	// Only do this if both clients agree on who's ahead!!
	if advantage >= radvantage {
		return int64(0)
	}

	// Both clients agree that we're the one ahead.  Split
	// the difference between the two to figure out how long to
	// sleep for.
	sleepFrames := int64(((radvantage - advantage) / 2) + 0.5)

	logrus.Info(fmt.Sprintf("iteration %d:  sleep frames is %d", t.Count, sleepFrames))

	// Some things just aren't worth correcting for.  Make sure
	// the difference is relevant before proceeding.
	if sleepFrames < MIN_FRAME_ADVANTAGE {
		return int64(0)
	}

	// Make sure our input had been "idle enough" before recommending
	// a sleep.  This tries to make the emulator sleep while the
	// user's input isn't sweeping in arcs (e.g. fireball motions in
	// Street Fighter), which could cause the player to miss moves.
	if requireIdleInput {
		for i := 1; i < len(t.LastInputs); i++ {
			if !t.LastInputs[i].Equal(t.LastInputs[0], true) {
				logrus.Info(fmt.Sprintf("iteration %d: rejecting due to input stuff at position %d !", t.Count, i))
				return int64(0)
			}
		}
	}

	// Success! Recommend the number of frames to sleep and adjust
	return MIN(sleepFrames, MAX_FRAME_ADVANTAGE)
}
