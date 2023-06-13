package thp

import (
	"runtime"
	"sync/atomic"

	"github.com/storozhukBM/thp/internal/goid"
)

// wideInt64 struct represents a 64-bit integer value,
// padded with an array of bytes to align it with the size of a cache line.
// This padding helps prevent false sharing, which can occur when
// multiple CPU cores access nearby memory locations simultaneously.
type wideInt64 struct {
	_ noCopy
	_ [cacheLineSize - 8]byte
	v atomic.Int64
}

// Counter is a concurrent counter implementation with striping,
// designed to enhance performance in write-heavy and contended workloads.
// It reduces contention by distributing the workload across multiple internal counters.
// Compared to the atomic.Int64 type, this counter may use more memory
// and have a slower Load operation.
// However, its Add operations scales better under high load and contention.
// To balance scalability and memory overhead, you can adjust the level of striping
// by using the NewCounterWithWideness function and specifying your desired wideness.
//
// NOTE: zero value of Counter is NOT valid, please create new counters using methods provided below.
type Counter struct {
	_             noCopy
	stripedValues []wideInt64
}

// NewCounter create new instance of Counter optimised for maximum scalability of write operations.
func NewCounter() *Counter {
	return NewCounterWithWideness(runtime.NumCPU())
}

// NewCounterWithWideness creates new instance of Counter with specified wideness.
// Using this method you can balance scalability and memory overhead.
func NewCounterWithWideness(wideness int) *Counter {
	if wideness > runtime.NumCPU() || wideness <= 0 {
		wideness = runtime.NumCPU()
	}
	n := nextHighestPowerOf2(int32(wideness))
	//nolint:exhaustruct // we skip explicit init of _ fields
	return &Counter{
		stripedValues: make([]wideInt64, n),
	}
}

// Add atomically adds x to current Counter value.
func (c *Counter) Add(x int64) {
	// put our value into stripped slice of values
	localStripeIdx := (len(c.stripedValues) - 1) & int(goid.ID())
	c.stripedValues[localStripeIdx].v.Add(x)
}

// Load calculates current Counter value, but can omit concurrent updates that happen during Load.
func (c *Counter) Load() int64 {
	result := int64(0)
	for i := range c.stripedValues {
		result += c.stripedValues[i].v.Load()
	}
	return result
}

// Clear sets counter to 0.
func (c *Counter) Clear() {
	for i := range c.stripedValues {
		c.stripedValues[i].v.Store(0)
	}
}
