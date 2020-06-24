package lib

import "github.com/sirupsen/logrus"

type U interface{}

type StaticBuffer struct {
	Elements []*U
	Size     int64
	N        int64
}

func (r *StaticBuffer) Init(n int64) {
	r.Size = 0
	r.N = n
	r.Elements = make([]*U, r.N)
}

func (r *StaticBuffer) Get(i int64) *U {
	if i < 0 || i > r.Size {
		logrus.Panic("Assert error : get out of range in staticbuffer")
	}
	return r.Elements[i]
}

func (r *StaticBuffer) PushBack(u *U) {
	if r.Size == r.N-1 {
		logrus.Panic("Assert error : StaticBuffer size pushback")
	}
	r.Elements[r.Size] = u
	r.Size++
}
