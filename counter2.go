package thp

import (
	"github.com/storozhukBM/thp/internal/goid"
	"runtime"
	"sync/atomic"
)

type wideInt2 struct {
	_ [cacheLineSize - 8]byte
	v atomic.Int64
}

type Counter2 struct {
	stripedValues []wideInt2
}

func NewCounter2(wideness int) *Counter2 {
	if wideness > runtime.NumCPU() || wideness <= 0 {
		wideness = runtime.NumCPU()
	}
	n := nextHighestPowerOf2(int32(wideness))
	return &Counter2{
		stripedValues: make([]wideInt2, n),
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
	for i := range c.stripedValues {
		result += c.stripedValues[i].v.Load()
	}
	return result
}
