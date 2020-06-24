package lib

type Poll struct {
	LoopSinks StaticBuffer
}

type IPollSink interface {
	OnLoopPoll() bool
}

type PollSinkCb struct {
	Sink *IPollSink
}

func (p *PollSinkCb) Init(s *IPollSink) {
	p.Sink = s
}

func (p *Poll) Init() {
	p.LoopSinks.Init(16)
}

func (p *Poll) RegisterLoop(sink *IPollSink) {
	var pollSink PollSinkCb
	pollSink.Init(sink)
	var u U = &pollSink
	p.LoopSinks.PushBack(&u)
}

func (p *Poll) Pump() bool {
	finished := false
	var i int64
	for i = 0; i < p.LoopSinks.Size; i++ {
		var cb *PollSinkCb = (*p.LoopSinks.Get(i)).(*PollSinkCb)
		var s IPollSink = *cb.Sink
		finished = !s.OnLoopPoll() || finished
	}

	return finished
}
