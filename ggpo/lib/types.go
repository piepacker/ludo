package lib

func MIN(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

func MAX(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

const MAX_INT = 0xEFFFFFF //TODO: Usefull?? Max int64?
