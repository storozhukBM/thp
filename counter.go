package thp

import (
	"github.com/storozhukBM/thp/internal/goid"
	"runtime"
	"sync/atomic"
)

var _n = 0

func init() {
	n := int32(runtime.NumCPU())
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n++
	_n = int(n)
}

type wideInt struct {
	_ cacheLinePadding
	v atomic.Int64
}

type Counter struct {
	stripedValues []wideInt
}

func NewCounter() *Counter {
	return &Counter{
		stripedValues: make([]wideInt, _n),
	}
}

// Add adds x to current counter value
func (c *Counter) Add(x int64) {
	// put our value into stripped slice of values
	localStripeIdx := (_n - 1) & int(goid.ID())
	c.stripedValues[localStripeIdx].v.Add(x)
}

func (c *Counter) Load() int64 {
	result := int64(0)
	for i, _ := range c.stripedValues {
		result += c.stripedValues[i].v.Load()
	}
	return result
}
