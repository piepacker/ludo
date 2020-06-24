package delay

import (
	"log"

	"github.com/libretro/ludo/netplay"
)

var RemoteInputQueue chan [20]bool

var Count uint64

const Delay = 10

func init() {
	RemoteInputQueue = make(chan [20]bool, 60)
}

func ReceiveInputs() {
	for {
		log.Println("receive inputs")

		netinput := [20]byte{}
		if _, err := netplay.Conn.Read(netinput[:]); err != nil {
			log.Fatalln(err)
		}

		Count++
		log.Println("incr", Count)

		log.Println(netinput)

		playerInput := [20]bool{}
		for i, b := range netinput {
			if b == 1 {
				playerInput[i] = true
			}
		}

		RemoteInputQueue <- playerInput
	}
}
