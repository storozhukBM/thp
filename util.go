package thp

const cacheLineSize = 64

// noCopy implements sync.Locker so that go vet can trigger
// warnings when types embedding noCopy are copied.
type noCopy struct{}

func (c *noCopy) Lock()   {}
func (c *noCopy) Unlock() {}

func zero[T any]() T {
	return *new(T)
}

//nolint:gomnd // pure magic, described here https://graphics.stanford.edu/~seander/bithacks.html#RoundUpPowerOf2
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
