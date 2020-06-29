package netplay

import (
	"fmt"
	"math/rand"

	"github.com/libretro/ludo/ggpo"
	"github.com/libretro/ludo/ggpo/ggponet"
	local "github.com/libretro/ludo/input"
	"github.com/sirupsen/logrus"
)

var ggpoSession *ggponet.GGPOSession = nil
var ngs NonGameState = NonGameState{}
var syncTest = false

const FRAME_DELAY = 2 //TODO: Make frame delay depends on local network connection

func Init(numPlayers int64, players []ggponet.GGPOPlayer, localPort string, numSpectators int64, test bool) {
	var result ggponet.GGPOErrorCode
	syncTest = test

	// Initialize the game state
	ngs.NumPlayers = numPlayers

	// Fill in a ggpo callbacks structure to pass to start_session.
	var cb ggponet.GGPOSessionCallbacks = &Callbacks{}

	if syncTest {
		//result = ggpo.StartSynctest(&ggpoSession, &cb, "ludo", num_players, sizeof(int), 1)
	} else {
		//TODO: Define optimal input size (default ActionLast)
		result = ggpo.StartSession(&ggpoSession, cb, "ludo", numPlayers, int64(local.ActionLast), localPort)
	}

	// automatically disconnect clients after 3000 ms and start our count-down timer
	// for disconnects after 1000 ms.   To completely disable disconnects, simply use
	// a value of 0 for ggpo_set_disconnect_timeout.
	ggpo.SetDisconnectTimeout(ggpoSession, 3000)
	ggpo.SetDisconnectNotifyStart(ggpoSession, 1000)

	for i := 0; i < int(numPlayers+numSpectators); i++ {
		var handle ggponet.GGPOPlayerHandle
		result = ggpo.AddPlayer(ggpoSession, &players[i], &handle)
		ngs.Players[i].Handle = handle
		ngs.Players[i].Type = players[i].Type
		if players[i].Type == ggponet.GGPO_PLAYERTYPE_LOCAL {
			ngs.Players[i].ConnectProgress = 100
			ngs.LocalPlayerHandle = handle
			ngs.SetConnectState(handle, Connecting)
			ggpo.SetFrameDelay(ggpoSession, handle, FRAME_DELAY)
		} else {
			ngs.Players[i].ConnectProgress = 0
		}
	}

	if result != ggponet.GGPO_OK {
		logrus.Panic("Error during Network Init")
	}
}

func InitSpectator(numPlayers int64, hostIp string, hostPort uint64) {
	//TODO: Spectators
	//var result ggponet.GGPOErrorCode

	// Initialize the game state
	//gs.Init(hwnd, num_players);
	ngs.NumPlayers = numPlayers

	// Fill in a ggpo callbacks structure to pass to start_session.
	//var cb ggponet.GGPOSessionCallbacks = &Callbacks{}

	//result = ggpo_start_spectating(&ggpo, &cb, "vectorwar", num_players, sizeof(int), localport, host_ip, host_port)
}

func DisconnectPlayer(player int64) {
	if player < ngs.NumPlayers {
		var result ggponet.GGPOErrorCode = ggpo.DisconnectPlayer(ggpoSession, ngs.Players[player].Handle)
		if ggponet.GGPO_SUCCEEDED(result) {
			logrus.Info(fmt.Sprintf("Disconnected player %d", player))
		} else {
			logrus.Error(fmt.Sprintf("Error while disconnecting player (err:%d)", result))
		}
	}
}

func AdvanceFrame(inputs []byte, disconnectFlags int64) {
	if len(inputs) == int(local.ActionLast*ggponet.GGPO_MAX_PLAYERS) {
		playersInputs := make([][]byte, ggponet.GGPO_MAX_PLAYERS)
		for i := 0; i < len(inputs); i += int(local.ActionLast) {
			playersInputs[i/int(local.ActionLast)] = make([]byte, local.ActionLast)
			for j := 0; j < int(local.ActionLast); j++ {
				playersInputs[i/int(local.ActionLast)][j] = inputs[i+j]
			}
		}
		for i := 0; i < len(playersInputs); i++ {
			local.NewState[i] = ByteToBool(playersInputs[i])
		}
		logrus.Info("======================= Inputs : ", inputs)
		logrus.Info("======================= NewState[0] : ", local.NewState[0])
		logrus.Info("======================= NewState[1] : ", local.NewState[1])
		logrus.Info("======================= NewState[2] : ", local.NewState[2])
		logrus.Info("======================= NewState[3] : ", local.NewState[3])
	}

	// Notify ggpo that we've moved forward exactly 1 frame.
	ggpo.AdvanceFrame(ggpoSession)

	// Update the performance monitor display.
	var handles [MAX_PLAYERS]ggponet.GGPOPlayerHandle
	count := 0
	for i := 0; i < int(ngs.NumPlayers); i++ {
		if ngs.Players[i].Type == ggponet.GGPO_PLAYERTYPE_REMOTE {
			handles[count] = ngs.Players[i].Handle
			count++
		}
	}
}

func BoolToByte(inputs [local.ActionLast]bool) []byte {
	byteInputs := make([]byte, len(inputs))
	for i, b := range inputs {
		if b {
			byteInputs[i] = 1
		}
	}
	return byteInputs
}

func ByteToBool(inputs []byte) [local.ActionLast]bool {
	boolInputs := [local.ActionLast]bool{}
	for i, b := range inputs {
		if b == 1 {
			boolInputs[i] = true
		}
	}
	return boolInputs
}

func RandBoolSlice() [local.ActionLast]bool {
	boolInputs := [local.ActionLast]bool{}
	for i := 0; i < int(local.ActionLast); i++ {
		if rand.Intn(1) == 1 {
			boolInputs[i] = true
		}
	}
	return boolInputs
}

func RunFrame() {
	var result ggponet.GGPOErrorCode = ggponet.GGPO_OK
	var disconnectFlags int64
	inputs := make([]byte, int64(local.ActionLast*ggponet.GGPO_MAX_PLAYERS))

	local.Reset()
	local.NewState = local.PollJoypads(local.NewState)
	local.NewState = local.PollKeyboard(local.NewState)

	if ngs.LocalPlayerHandle != ggponet.GGPO_INVALID_HANDLE {
		input := BoolToByte(local.NewState[0])
		if syncTest {
			input = BoolToByte(RandBoolSlice()) // test: use random inputs to demonstrate sync testing
		}
		result = ggpo.AddLocalInput(ggpoSession, ngs.LocalPlayerHandle, input, int64(len(input)))
	}

	// synchronize these inputs with ggpo.  If we have enough input to proceed
	// ggpo will modify the input list with the correct inputs to use and
	// return 1.
	if ggponet.GGPO_SUCCEEDED(result) {
		result = ggpo.SynchronizeInput(ggpoSession, inputs, int64(local.ActionLast*ggponet.GGPO_MAX_PLAYERS), &disconnectFlags)
		if ggponet.GGPO_SUCCEEDED(result) {
			// inputs[0] and inputs[1] contain the inputs for p1 and p2.  Advance
			// the game by 1 frame using those inputs.
			AdvanceFrame(inputs, disconnectFlags)
		}
	}
}

func Idle() {
	ggpo.Idle(ggpoSession)
}
