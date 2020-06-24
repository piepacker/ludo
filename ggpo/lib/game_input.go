package lib

import (
	"bytes"
	"fmt"

	"github.com/sirupsen/logrus"
)

const (
	GAMEINPUT_MAX_BYTES   = 20 //TODO: Get this value dynamically (with ActionLast for example)
	GAMEINPUT_MAX_PLAYERS = 2
	NULL_FRAME            = -1
)

type GameInput struct {
	Size  int64
	Frame int64
	Bits  []byte
}

func (g *GameInput) Init(iframe int64, ibits []byte, isize int64, offset int64) {
	if isize > GAMEINPUT_MAX_BYTES*GAMEINPUT_MAX_PLAYERS || isize == 0 {
		logrus.Panic(fmt.Sprintf("Size Error, isize = %d", isize))
	}
	g.Frame = iframe
	g.Size = isize
	g.Bits = make([]byte, GAMEINPUT_MAX_BYTES)
	if len(ibits) > 0 {
		for k := 0; k < int(offset*isize); k += int(isize) {
			for j := 0; j < int(isize); j++ {
				g.Bits[k+j] = ibits[j]
			}
		}
	}
}

func (g *GameInput) SimpleInit(iframe int64, ibits []byte, isize int64) {
	if isize > GAMEINPUT_MAX_BYTES*GAMEINPUT_MAX_PLAYERS || isize == 0 {
		logrus.Panic(fmt.Sprintf("Size Error, isize = %d", isize))
	}
	g.Frame = iframe
	g.Size = isize
	g.Bits = make([]byte, GAMEINPUT_MAX_BYTES)
	if len(ibits) > 0 {
		copy(g.Bits, ibits)
	}
}

func (g *GameInput) Equal(other GameInput, bitsonly bool) bool {
	if !bitsonly && g.Frame != other.Frame {
		logrus.Info(fmt.Sprintf("frames don't match: %d, %d", g.Frame, other.Frame))
	}

	if g.Size != other.Size {
		logrus.Info(fmt.Sprintf("sizes don't match: %d, %d", g.Size, other.Size))
	}

	if bytes.Compare(g.Bits, other.Bits) != 0 {
		logrus.Info("bits don't match")
	}

	if g.Size != other.Size {
		logrus.Panic("sizes don't match")
	}

	return (bitsonly || g.Frame == other.Frame) && g.Size == other.Size && bytes.Compare(g.Bits, other.Bits) == 0
}

func (g *GameInput) Set(i int64) {
	g.Bits[i/8%(GAMEINPUT_MAX_BYTES)] |= (1 << (i % 8))
}

func (g *GameInput) Clear(i int64) {
	g.Bits[i/8%(GAMEINPUT_MAX_BYTES)] &= ^(1 << (i % 8))
}

func (g *GameInput) Erase() {
	g.Bits = make([]byte, len(g.Bits))
}

func (g *GameInput) Value(i int64) bool {
	return (g.Bits[i/8%(GAMEINPUT_MAX_BYTES)] & (1 << (i % 8))) != 0
}
