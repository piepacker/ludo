package network

import "github.com/libretro/ludo/ggpo/lib"

type TypeEvent int64

const (
	EventUnknown            TypeEvent = -1
	EventConnected          TypeEvent = 0
	EventSynchronizing      TypeEvent = 1
	EventSynchronized       TypeEvent = 2
	EventInput              TypeEvent = 3
	EventDisconnected       TypeEvent = 4
	EventNetworkInterrupted TypeEvent = 5
	EventNetworkResumed     TypeEvent = 6
	EventGameState          TypeEvent = 7
)

type Synchronizing struct {
	Total int64
	Count int64
}

type Event struct {
	Type              TypeEvent
	Input             lib.GameInput
	SavedFrame        lib.SavedFrame
	Synchronizing     Synchronizing
	DisconnectTimeout int64
}

func (e *Event) Init(t TypeEvent) {
	e.Type = t
}
