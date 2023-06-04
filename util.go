package thp

const cacheLineSize = 64

type cacheLinePadding [cacheLineSize]byte

func zero[T any]() T {
	return *new(T)
}

func ptr[T any](v T) *T {
	return &v
}

func nextHighestPowerOf2(wideness int32) int32 {
	n := wideness
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	return n
}
