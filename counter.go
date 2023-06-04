package thp

import (
	"runtime"
	"sync/atomic"

	"github.com/storozhukBM/thp/internal/goid"
)

type wideInt struct {
	_ cacheLinePadding
	v atomic.Int64
}

type Counter struct {
	stripedValues []wideInt
}

func NewCounter(wideness int) *Counter {
	if wideness > runtime.NumCPU() || wideness <= 0 {
		wideness = runtime.NumCPU()
	}
	n := nextHighestPowerOf2(int32(wideness))
	return &Counter{
		stripedValues: make([]wideInt, n),
	}
}

// Add adds x to current counter value
func (c *Counter) Add(x int64) {
	// put our value into stripped slice of values
	localStripeIdx := (len(c.stripedValues) - 1) & int(goid.ID())
	c.stripedValues[localStripeIdx].v.Add(x)
}

func (c *Counter) Load() int64 {
	result := int64(0)
	for i := range c.stripedValues {
		result += c.stripedValues[i].v.Load()
	}
	return result
}
