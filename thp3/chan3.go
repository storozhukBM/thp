package thp3

import (
	"context"
	"runtime"
	"sync"
)

const ErrChanBatchSize chanError = "Batch size for thp.Chan can't be lower than 1"

type Chan[T any] struct {
	batchSize    int
	internalChan chan *batch[T]
	batchPool    sync.Pool
}

// We use pointer to a batch instead of regular slice, to place it into sync.Pool.
// This helps us to avoid extra allocation when storing it as interface{}/any.
type batch[T any] struct {
	buf []T
}

func NewChan[T any](batchSize int) (*Chan[T], func()) {
	if batchSize < 1 {
		panic(ErrChanBatchSize)
	}

	ch := &Chan[T]{
		batchSize:    batchSize,
		internalChan: make(chan *batch[T], runtime.NumCPU()),
		batchPool: sync.Pool{
			New: func() any {
				// We use pointer to a batch instead of regular slice, to place it into sync.Pool.
				// This helps us to avoid extra allocation when storing it as interface{}/any.
				return &batch[T]{buf: make([]T, 0, batchSize)}
			},
		},
	}

	return ch, ch.Close
}

func (ch *Chan[T]) Close() {
	close(ch.internalChan)
}

// We use pointer to a batch instead of regular slice, to place it into sync.Pool.
// This helps us to avoid extra allocation when storing it as interface{}/any.
func (ch *Chan[T]) getBatchFromPool() *batch[T] {
	//nolint:forcetypeassert // panic on type missmatch is fine here
	return ch.batchPool.Get().(*batch[T])
}

// We use pointer to a batch instead of regular slice, to place it into sync.Pool.
// This helps us to avoid extra allocation when storing it as interface{}/any.
func (ch *Chan[T]) putBatchToPool(batch *batch[T]) {
	batch.buf = batch.buf[:0]
	ch.batchPool.Put(batch)
}

func (ch *Chan[T]) Producer(ctx context.Context) (*Producer[T], func()) {
	initialBatch := ch.getBatchFromPool()
	result := &Producer[T]{
		parent: ch,
		ctx:    ctx,
		batch:  initialBatch,
		buf:    initialBatch.buf,
	}
	return result, result.Flush
}

type Producer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	batch  *batch[T]
	// We unpack the batch.buf into a separate field to avoid extra memory hop
	// every time we access it, this yields significant speedup on our tests.
	// But we still need to store pointer to a batch to avoid allocations when working with sync.Pool.
	buf []T
}

func (p *Producer[T]) NonBlockingFlush() bool {
	if len(p.buf) == 0 || p.ctx.Err() != nil {
		return false
	}
	// TODO: write documentation on how to avoid items dropping
	// in case of ctx cancellation using detached context
	p.batch.buf = p.buf
	select {
	case p.parent.internalChan <- p.batch:
		p.batch = p.parent.getBatchFromPool()
		p.buf = p.batch.buf
		return true
	default:
		return false
	}
}

// Flush if you call Flush after producer context is canceled,
// Flush won't block, but it is possible that it will send data over the channel.
func (p *Producer[T]) Flush() {
	if len(p.buf) == 0 || p.ctx.Err() != nil {
		return
	}
	// TODO: write documentation on how to avoid items dropping
	// in case of ctx cancellation using detached context
	p.batch.buf = p.buf
	select {
	case p.parent.internalChan <- p.batch:
		p.batch = p.parent.getBatchFromPool()
		p.buf = p.batch.buf
	case <-p.ctx.Done():
		// we can't block this goroutine anymore
		// and will drop batched items
		p.batch = &batch[T]{}
		p.buf = nil
	}
}

func (p *Producer[T]) Put(v T) {
	p.buf = append(p.buf, v)
	if len(p.buf) >= p.parent.batchSize {
		p.Flush()
	}
}

func (p *Producer[T]) NonBlockingPut(v T) bool {
	p.buf = append(p.buf, v)
	if len(p.buf) >= p.parent.batchSize {
		flush := p.NonBlockingFlush()
		return flush
	}
	return true
}

type Consumer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	idx    int
	batch  *batch[T]
	// We unpack the batch.buf into a separate field to avoid extra memory hop
	// every time we access it, this yields significant speedup on our tests.
	// But we still need to store pointer to a batch to avoid allocations when working with sync.Pool.
	buf []T
}

func (ch *Chan[T]) Consumer(ctx context.Context) *Consumer[T] {
	result := &Consumer[T]{
		ctx:    ctx,
		parent: ch,
		idx:    0,
		batch:  &batch[T]{},
		buf:    nil,
	}
	return result
}

func (c *Consumer[T]) prefetch() bool {
	if cap(c.buf) > 0 {
		c.parent.putBatchToPool(c.batch)
	}
	c.idx = 0
	c.batch = nil
	c.buf = nil
	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		if batch != nil {
			c.buf = batch.buf
		}
		return ok
	case <-c.ctx.Done():
		return false
	}
}

//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two returned booleans
func (c *Consumer[T]) nonBlockingPrefetch() (readSuccess bool, channelIsOpen bool) {
	if cap(c.buf) > 0 {
		c.parent.putBatchToPool(c.batch)
	}
	c.idx = 0
	c.batch = nil
	c.buf = nil
	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		if batch != nil {
			c.buf = batch.buf
		}
		return ok, ok
	default:
		return false, true
	}
}

func (c *Consumer[T]) Poll() (T, bool) {
	if c.idx >= len(c.buf) {
		ok := c.prefetch()
		if !ok {
			return zero[T](), false
		}
	}
	item := c.buf[c.idx]
	c.idx++
	return item, true
}

//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two last returned booleans
func (c *Consumer[T]) NonBlockingPoll() (value T, readSuccess bool, channelIsOpen bool) {
	if c.idx >= len(c.buf) {
		success, open := c.nonBlockingPrefetch()
		if !success {
			return zero[T](), success, open
		}
	}
	item := c.buf[c.idx]
	c.idx++
	return item, true, true
}

func zero[T any]() T {
	return *new(T)
}

type chanError string

func (m chanError) Error() string {
	return string(m)
}
