package ggponet

const (
	GGPO_MAX_PLAYERS              = 4
	GGPO_MAX_PREDICTION_FRAMES    = 8
	GGPO_MAX_SPECTATORS           = 32
	GGPO_SPECTATOR_INPUT_INTERVAL = 4
)

type GGPOSession interface {
	DoPoll() GGPOErrorCode
	AddPlayer(player *GGPOPlayer, handle *GGPOPlayerHandle) GGPOErrorCode
	AddLocalInput(player GGPOPlayerHandle, values []byte, size int64) GGPOErrorCode
	SyncInput(values []byte, size int64, disconnectFlags *int64) GGPOErrorCode
	IncrementFrame() GGPOErrorCode
	//Chat(text string) GGPOErrorCode
	DisconnectPlayer(handle GGPOPlayerHandle) GGPOErrorCode
	GetNetworkStats(stats *GGPONetworkStats, handle GGPOPlayerHandle) GGPOErrorCode
	//Logv(fmt string, va_list list) GGPOErrorCode
	SetFrameDelay(player GGPOPlayerHandle, delay int64) GGPOErrorCode
	SetDisconnectTimeout(timeout int64) GGPOErrorCode
	SetDisconnectNotifyStart(timeout int64) GGPOErrorCode
}

type GGPOPlayerHandle int64

type GGPOPlayerType int64

const (
	GGPO_PLAYERTYPE_LOCAL GGPOPlayerType = iota
	GGPO_PLAYERTYPE_REMOTE
	GGPO_PLAYERTYPE_SPECTATOR
)

type GGPOPlayer struct {
	Size      int64
	Type      GGPOPlayerType
	PlayerNum int64
	IPAddress string
	Port      uint64
}

type GGPOLocalEndpoint struct {
	PlayerNum int64
}

type GGPOErrorCode int64

const (
	GGPO_OK                              GGPOErrorCode = 0
	GGPO_ERRORCODE_SUCCESS               GGPOErrorCode = 0
	GGPO_ERRORCODE_GENERAL_FAILURE       GGPOErrorCode = -1
	GGPO_ERRORCODE_INVALID_SESSION       GGPOErrorCode = 1
	GGPO_ERRORCODE_INVALID_PLAYER_HANDLE GGPOErrorCode = 2
	GGPO_ERRORCODE_PLAYER_OUT_OF_RANGE   GGPOErrorCode = 3
	GGPO_ERRORCODE_PREDICTION_THRESHOLD  GGPOErrorCode = 4
	GGPO_ERRORCODE_UNSUPPORTED           GGPOErrorCode = 5
	GGPO_ERRORCODE_NOT_SYNCHRONIZED      GGPOErrorCode = 6
	GGPO_ERRORCODE_IN_ROLLBACK           GGPOErrorCode = 7
	GGPO_ERRORCODE_INPUT_DROPPED         GGPOErrorCode = 8
	GGPO_ERRORCODE_PLAYER_DISCONNECTED   GGPOErrorCode = 9
	GGPO_ERRORCODE_TOO_MANY_SPECTATORS   GGPOErrorCode = 10
	GGPO_ERRORCODE_INVALID_REQUEST       GGPOErrorCode = 11
)

func GGPO_SUCCEEDED(result GGPOErrorCode) bool {
	return (result) == GGPO_ERRORCODE_SUCCESS
}

const GGPO_INVALID_HANDLE = -1

type GGPOEventCode int64

const (
	GGPO_EVENTCODE_CONNECTED_TO_PEER       GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_SYNCHRONIZING_WITH_PEER GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_SYNCHRONIZED_WITH_PEER  GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_RUNNING                 GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_DISCONNECTED_FROM_PEER  GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_TIMESYNC                GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_CONNECTION_INTERRUPTED  GGPOEventCode = iota + 1000
	GGPO_EVENTCODE_CONNECTION_RESUMED      GGPOEventCode = iota + 1000
)

type connected struct {
	Player GGPOPlayerHandle
}

type synchronizing struct {
	Player GGPOPlayerHandle
	Count  int64
	Total  int64
}

type synchronized struct {
	Player GGPOPlayerHandle
}

type disconnected struct {
	Player GGPOPlayerHandle
}

type timesync struct {
	FramesAhead int64
}

type connectionInterrupted struct {
	Player            GGPOPlayerHandle
	DisconnectTimeout int64
}

type connectionResumed struct {
	Player GGPOPlayerHandle
}

type GGPOEvent struct {
	Code                  GGPOEventCode
	Connected             connected
	Synchronizing         synchronizing
	Synchronized          synchronized
	Disconnected          disconnected
	TimeSync              timesync
	ConnectionInterrupted connectionInterrupted
	ConnectionResumed     connectionResumed
}

type network struct {
	SendQueueLen int64
	RecvQueueLen int64
	Ping         int64
	KbpsSent     int64
}

type sync struct {
	LocalFramesBehind  int64
	RemoteFramesBehind int64
}

/*
 * The GGPOSessionCallbacks structure contains the callback functions that
 * your application must implement. GGPO.net will periodically call these
 * functions during the game. All callback functions must be implemented.
 */
type GGPOSessionCallbacks interface {
	BeginGame(game string) bool
	SaveGameState(buffer []byte, len *int64, checksum *int64, frame int64)
	LoadGameState(buffer []byte, len int64)
	LogGameState(filename string, buffer *byte, len int64)
	AdvanceFrame(flags int64)
	OnEvent(info *GGPOEvent)
}

type GGPONetworkStats struct {
	Network  network
	TimeSync sync
}

type ConnectStatus struct {
	Disconnected bool
	LastFrame    int64
}
