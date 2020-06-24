package lib

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

const (
	INPUT_QUEUE_LENGTH = 128
	DEFAULT_INPUT_SIZE = 4
)

func PREVIOUS_FRAME(offset int64) int64 {
	if offset == 0 {
		return INPUT_QUEUE_LENGTH - 1
	}
	return offset - 1
}

type InputQueue struct {
	ID                  int64
	Head                int64
	Tail                int64
	Length              int64
	FirstFrame          bool
	LastUserAddedFrame  int64
	LastAddedFrame      int64
	FirstIncorrectFrame int64
	LastFrameRequested  int64
	FrameDelay          int64
	Inputs              []GameInput
	Prediction          GameInput
}

func (i *InputQueue) Init(id int64, inputSize int64) {
	i.ID = id
	i.Head = 0
	i.Tail = 0
	i.Length = 0
	i.FrameDelay = 0
	i.FirstFrame = true
	i.LastUserAddedFrame = NULL_FRAME
	i.FirstIncorrectFrame = NULL_FRAME
	i.LastFrameRequested = NULL_FRAME
	i.LastAddedFrame = NULL_FRAME
	i.Inputs = make([]GameInput, INPUT_QUEUE_LENGTH)

	i.Prediction.SimpleInit(NULL_FRAME, nil, inputSize)

	for j := 0; j < len(i.Inputs); j++ {
		i.Inputs[j].Size = inputSize
	}
}

func (i *InputQueue) GetLastConfirmedFrame() int64 {
	logrus.Info(fmt.Sprintf("returning last confirmed frame %d.", i.LastAddedFrame))
	return i.LastAddedFrame
}

func (i *InputQueue) GetFirstIncorrectFrame() int64 {
	logrus.Info(fmt.Sprintf("returning first incorrect frame %d.", i.FirstIncorrectFrame))
	return i.FirstIncorrectFrame
}

func (i *InputQueue) SetFrameDelay(delay int64) {
	i.FrameDelay = delay
}

func (i *InputQueue) DiscardConfirmedFrames(frame int64) {

	if frame < 0 {
		logrus.Panic("frame < 0")
	}

	if i.LastFrameRequested != NULL_FRAME {
		frame = MIN(frame, i.LastFrameRequested)
	}

	logrus.Info(fmt.Sprintf("discarding confirmed frames up to %d (last_added:%d length:%d [head:%d tail:%d]).",
		frame, i.LastAddedFrame, i.Length, i.Head, i.Tail))

	if frame >= i.LastAddedFrame {
		i.Tail = i.Head
	} else {
		offset := frame - i.Inputs[i.Tail].Frame + 1

		logrus.Info(fmt.Sprintf("difference of %d frames.", offset))

		// if offset < 0 {
		// 	logrus.Panic("offset < 0")
		// }

		i.Tail = (i.Tail + offset) % INPUT_QUEUE_LENGTH
		i.Length -= offset
	}

	logrus.Info(fmt.Sprintf("after discarding, new tail is %d (frame:%d).", i.Tail, i.Inputs[i.Tail].Frame))
	if i.Length < 0 {
		logrus.Panic("Length < 0")
	}
}

func (i *InputQueue) ResetPrediction(frame int64) {

	if i.FirstIncorrectFrame == NULL_FRAME && frame <= i.FirstIncorrectFrame {
		logrus.Panic(fmt.Sprintf("Assert Error on FirstIncorrectFrame"))
	}

	logrus.Info(fmt.Sprintf("resetting all prediction errors back to frame %d.", frame))

	/*
	 * There's nothing really to do other than reset our prediction
	 * state and the incorrect frame counter...
	 */
	i.Prediction.Frame = NULL_FRAME
	i.FirstIncorrectFrame = NULL_FRAME
	i.LastFrameRequested = NULL_FRAME
}

func (i *InputQueue) GetConfirmedInput(requestedFrame int64, input *GameInput) bool {

	if i.FirstIncorrectFrame == NULL_FRAME && requestedFrame <= i.FirstIncorrectFrame {
		logrus.Panic(fmt.Sprintf("Assert Error on FirstIncorrectFrame"))
	}

	offset := requestedFrame % INPUT_QUEUE_LENGTH
	if i.Inputs[offset].Frame != requestedFrame {
		return false
	}
	*input = i.Inputs[offset]
	return true
}

func (i *InputQueue) GetInput(requestedFrame int64, input *GameInput) bool {

	logrus.Info(fmt.Sprintf("requesting input frame %d.", requestedFrame))

	/*
	 * No one should ever try to grab any input when we have a prediction
	 * error.  Doing so means that we're just going further down the wrong
	 * path.  ASSERT this to verify that it's true.
	 */
	// if i.FirstIncorrectFrame != NULL_FRAME {
	// 	logrus.Panic("No one should ever try to grab any input when we have a prediction error.")
	// }

	// Remember the last requested frame number for later.  We'll need this in AddInput() to drop out of prediction mode.
	i.LastFrameRequested = requestedFrame

	// if requestedFrame < i.Inputs[i.Tail].Frame {
	// 	logrus.Panic("Assert Error requestedFrame : ", requestedFrame, " < i.Inputs[i.Tail].Frame : ", i.Inputs[i.Tail].Frame)
	// }

	if i.Prediction.Frame == NULL_FRAME {
		//If the frame requested is in our range, fetch it out of the queue and return it.
		var offset int64 = requestedFrame - i.Inputs[i.Tail].Frame

		if offset < i.Length {
			offset = (offset + i.Tail) % INPUT_QUEUE_LENGTH

			if i.Inputs[offset].Frame != requestedFrame {
				logrus.Panic("Assert Error on requestedFrame")
			}

			*input = i.Inputs[offset]

			logrus.Info(fmt.Sprintf("returning confirmed frame number %d.", input.Frame))

			return true
		}

		/*
		 * The requested frame isn't in the queue.  Bummer.  This means we need
		 * to return a prediction frame.  Predict that the user will do the
		 * same thing they did last time.
		 */

		if requestedFrame == 0 {
			logrus.Info("basing new prediction frame from nothing, you're client wants frame 0.")
			i.Prediction.Erase()
		} else if i.LastAddedFrame == NULL_FRAME {
			logrus.Info("basing new prediction frame from nothing, since we have no frames yet.")
			i.Prediction.Erase()
		} else {
			logrus.Info(fmt.Sprintf("basing new prediction frame from previously added frame (queue entry:%d, frame:%d).",
				PREVIOUS_FRAME(i.Head), i.Inputs[PREVIOUS_FRAME(i.Head)].Frame))
			i.Prediction = i.Inputs[PREVIOUS_FRAME(i.Head)]
		}
		i.Prediction.Frame++
	}

	if i.Prediction.Frame < 0 {
		logrus.Panic("Assert Error on Prediction.Frame")
	}

	/*
	 * If we've made it this far, we must be predicting.  Go ahead and
	 * forward the prediction frame contents.  Be sure to return the
	 * frame number requested by the client, though.
	 */

	*input = i.Prediction
	input.Frame = requestedFrame
	logrus.Info(fmt.Sprintf("returning prediction frame number %d (%d).", input.Frame, i.Prediction.Frame))

	return false
}

func (i *InputQueue) AddInput(input *GameInput) {
	var new_frame int64

	logrus.Info(fmt.Sprintf("adding input frame number %d to queue.", input.Frame))

	/*
	 * These next two lines simply verify that inputs are passed in
	 * sequentially by the user, regardless of frame delay.
	 */
	if i.LastUserAddedFrame != NULL_FRAME && input.Frame != i.LastUserAddedFrame+1 {
		logrus.Panic("Assert Error : this assert verify that inputs are passed in sequentially by the user, regardless of frame delay.")
	}
	i.LastUserAddedFrame = input.Frame

	/*
	 * Move the queue head to the correct point in preparation to
	 * input the frame into the queue.
	 */
	new_frame = i.AdvanceQueueHead(input.Frame)
	if new_frame != NULL_FRAME {
		i.AddDelayedInputToQueue(*input, new_frame)
	}

	/*
	 * Update the frame number for the input.  This will also set the
	 * frame to GameInput::NullFrame for frames that get dropped (by
	 * design).
	 */
	input.Frame = new_frame
}

func (i *InputQueue) AddDelayedInputToQueue(input GameInput, frameNumber int64) {

	logrus.Info(fmt.Sprintf("adding delayed input frame number %d to queue.", frameNumber))

	if input.Size != i.Prediction.Size {
		logrus.Panic("Assert Error : Prediction Size")
	}

	if i.LastAddedFrame != NULL_FRAME && frameNumber != i.LastAddedFrame+1 {
		logrus.Panic("Assert Error : Frame")
	}

	if frameNumber != 0 && i.Inputs[PREVIOUS_FRAME(i.Head)].Frame != frameNumber-1 {
		logrus.Panic("Assert Error : Frame")
	}

	/*
	 * Add the frame to the back of the queue
	 */
	i.Inputs[i.Head] = input
	i.Inputs[i.Head].Frame = frameNumber
	i.Head = (i.Head + 1) % INPUT_QUEUE_LENGTH
	i.Length++
	i.FirstFrame = false

	i.LastAddedFrame = frameNumber

	if i.Prediction.Frame != NULL_FRAME {
		if frameNumber != i.Prediction.Frame {
			logrus.Panic("Assert error Prediction frame")
		}

		/*
		 * We've been predicting...  See if the inputs we've gotten match
		 * what we've been predicting.  If so, don't worry about it.  If not,
		 * remember the first input which was incorrect so we can report it
		 * in GetFirstIncorrectFrame()
		 */
		if i.FirstIncorrectFrame == NULL_FRAME && !i.Prediction.Equal(input, true) {
			logrus.Info(fmt.Sprintf("Frame %d does not match prediction. Marking error.", frameNumber))
			i.FirstIncorrectFrame = frameNumber
		}

		/*
		 * If this input is the same frame as the last one requested and we
		 * still haven't found any mis-predicted inputs, we can dump out
		 * of predition mode entirely!  Otherwise, advance the prediction frame
		 * count up.
		 */
		if i.Prediction.Frame == i.LastFrameRequested && i.FirstIncorrectFrame == NULL_FRAME {
			logrus.Info("prediction is correct!  dumping out of prediction mode.")
			i.Prediction.Frame = NULL_FRAME
		} else {
			i.Prediction.Frame++
		}
	}

	// if i.Length > INPUT_QUEUE_LENGTH {
	// 	logrus.Panic("Assert Error : Length too high")
	// }
}

func (i *InputQueue) AdvanceQueueHead(frame int64) int64 {

	logrus.Info(fmt.Sprintf("advancing queue head to frame %d.", frame))

	expectedFrame := i.Inputs[PREVIOUS_FRAME(i.Head)].Frame + 1
	if i.FirstFrame {
		expectedFrame = 0
	}

	frame += i.FrameDelay

	if expectedFrame > frame {
		/*
		 * This can occur when the frame delay has dropped since the last
		 * time we shoved a frame into the system.  In this case, there's
		 * no room on the queue.  Toss it.
		 */
		logrus.Info(fmt.Sprintf("Dropping input frame %d (expected next frame to be %d).", frame, expectedFrame))
		return NULL_FRAME
	}

	for expectedFrame < frame {
		/*
		 * This can occur when the frame delay has been increased since the last
		 * time we shoved a frame into the system.  We need to replicate the
		 * last frame in the queue several times in order to fill the space
		 * left.
		 */
		logrus.Info(fmt.Sprintf("Adding padding frame %d to account for change in frame delay.", expectedFrame))
		var lastFrame GameInput = i.Inputs[PREVIOUS_FRAME(i.Head)]
		i.AddDelayedInputToQueue(lastFrame, expectedFrame)
		expectedFrame++
	}

	if frame != 0 && frame != i.Inputs[PREVIOUS_FRAME(i.Head)].Frame+1 {
		logrus.Panic("Assert Error : Frame")
	}
	return frame
}
