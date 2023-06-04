package thp

import (
	"github.com/storozhukBM/thp/internal/goid"
	"runtime"
)

type Counter2 struct {
	stripedValues []wideInt
}

func NewCounter2(wideness int) *Counter2 {
	if wideness > runtime.NumCPU() || wideness <= 0 {
		wideness = runtime.NumCPU()
	}
	// compute the next highest power of 2 of 32-bit v
	n := int32(wideness)
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++

	return &Counter2{
		stripedValues: make([]wideInt, n),
	}
}

// Add adds x to current counter value
func (c *Counter2) Add(x int64) {
	// put our value into stripped slice of values
	localStripeIdx := (len(c.stripedValues) - 1) & int(goid.ID())
	c.stripedValues[localStripeIdx].v.Add(x)
}

func (c *Counter2) Load() int64 {
	result := int64(0)
	for i, _ := range c.stripedValues {
		result += c.stripedValues[i].v.Load()
	}
	return result
}
