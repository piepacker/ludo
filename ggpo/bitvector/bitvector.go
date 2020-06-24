package bitvector

const BITVECTOR_NIBBLE_SIZE = 9

func SetBit(vector []byte, offset *int64) {
	vector[(*offset)/8] |= (1 << ((*offset) % 8))
	*offset += 1
}

func ClearBit(vector []byte, offset *int64) {
	vector[(*offset)/8] &= ^(1 << ((*offset) % 8))
	*offset += 1
}

func WriteNibblet(vector []byte, nibble int64, offset *int64) {
	for i := 0; i < BITVECTOR_NIBBLE_SIZE; i++ {
		if nibble&(1<<i) > 0 {
			SetBit(vector, offset)
		} else {
			ClearBit(vector, offset)
		}
	}
}

func ReadBit(vector []byte, offset *int64) int64 {
	var retval int64 = int64(vector[(*offset)/8] & (1 << ((*offset) % 8)))
	if retval > 1 {
		retval = 1
	}
	*offset += 1
	return retval
}

func ReadNibblet(vector []byte, offset *int64) int64 {
	var nibblet int64 = 0
	for i := 0; i < BITVECTOR_NIBBLE_SIZE; i++ {
		nibblet |= (ReadBit(vector, offset) << i)
	}
	return nibblet
}
