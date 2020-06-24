package platform

import (
	"os"
	"strconv"
	"time"
)

func GetCurrentTimeMS() uint64 {
	return uint64(time.Now().UnixNano() / int64(time.Millisecond))
}

func GetConfigInt(key string) int64 {
	var result int
	var err error
	val, ok := os.LookupEnv(key)
	if !ok {
		result = 0
	} else {
		result, err = strconv.Atoi(val)
		if err != nil {
			result = 0
		}
	}
	return int64(result)
}
