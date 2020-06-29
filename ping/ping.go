package ping

import (
	"time"

	"github.com/sparrc/go-ping"
)

func GetAvgPing(addr string) int64 {
	pinger, err := ping.NewPinger(addr)
	if err != nil {
		panic(err)
	}
	pinger.Count = 3
	pinger.SetPrivileged(true) //For Windows, otherwise we get an error
	pinger.Run()
	return int64(pinger.Statistics().AvgRtt / time.Millisecond)
}
