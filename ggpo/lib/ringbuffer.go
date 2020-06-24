package lib

import "github.com/sirupsen/logrus"

type T interface{}

type RingBuffer struct {
	Elements []*T
	Head     int64
	Tail     int64
	Size     int64
	N        int64
}

func (r *RingBuffer) Init(n int64) {
	r.Head = 0
	r.Tail = 0
	r.Size = 0
	r.N = n
	r.Elements = make([]*T, r.N)
}

func (r *RingBuffer) Front() *T {
	if r.Size == r.N {
		logrus.Panic("Assert error ringbuffer size error")
	}
	return r.Elements[r.Tail]
}

func (r *RingBuffer) Item(i int64) *T {
	if i >= r.Size {
		logrus.Panic("Assert error ringbuffer size error")
	}
	return r.Elements[(r.Tail+i)%r.N]
}

func (r *RingBuffer) Pop() {
	if r.Size == r.N {
		logrus.Panic("Assert error ringbuffer size error")
	}
	r.Tail = (r.Tail + 1) % r.N
	r.Size--
}

func (r *RingBuffer) Push(t *T) {
	if r.Size == r.N-1 {
		logrus.Panic("Assert error ringbuffer size error")
	}
	r.Elements[r.Head] = t
	r.Head = (r.Head + 1) % r.N
	r.Size++
}

func (r *RingBuffer) Empty() bool {
	return r.Size == 0
}
