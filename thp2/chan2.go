package thp2

import (
	"context"
	"runtime"
	"sync"
)

const ErrChanBatchSize chanError = "Batch size for thp.Chan can't be lower than 1"

type Chan[T any] struct {
	batchSize    int
	internalChan chan *[]T
	batchPool    sync.Pool
}

func NewChan[T any](batchSize int) (*Chan[T], func()) {
	if batchSize < 1 {
		panic(ErrChanBatchSize)
	}

	ch := &Chan[T]{
		batchSize:    batchSize,
		internalChan: make(chan *[]T, runtime.NumCPU()),
		batchPool: sync.Pool{
			New: func() any {
				batch := make([]T, 0, batchSize)
				return &batch
			},
		},
	}

	return ch, ch.Close
}

func (ch *Chan[T]) Close() {
	close(ch.internalChan)
}

func (ch *Chan[T]) getBatchFromPool() *[]T {
	//nolint:forcetypeassert // panic on type missmatch is fine here
	return ch.batchPool.Get().(*[]T)
}

func (ch *Chan[T]) putBatchToPool(batch *[]T) {
	*batch = (*batch)[:0]
	ch.batchPool.Put(batch)
}

func (ch *Chan[T]) Producer(ctx context.Context) (*Producer[T], func()) {
	result := &Producer[T]{
		parent: ch,
		ctx:    ctx,
		batch:  ch.getBatchFromPool(),
	}

	return result, result.Flush
}

type Producer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	batch  *[]T
}

func (p *Producer[T]) NonBlockingFlush() bool {
	if len(*p.batch) == 0 || p.ctx.Err() != nil {
		return false
	}
	// TODO: write documentation on how to avoid items dropping
	// in case of ctx cancelation using detached context
	select {
	case p.parent.internalChan <- p.batch:
		p.batch = p.parent.getBatchFromPool()
		return true
	default:
		return false
	}
}

// Flush if you call Flush after producer context is canceled,
// Flush won't block but it is possible that it will send data over the channel.
func (p *Producer[T]) Flush() {
	if len(*p.batch) == 0 || p.ctx.Err() != nil {
		return
	}
	// TODO: write documentation on how to avoid items dropping
	// in case of ctx cancelation using detached context
	select {
	case p.parent.internalChan <- p.batch:
		p.batch = p.parent.getBatchFromPool()
	case <-p.ctx.Done():
		// we can't block this goroutine anymore
		// and will drop batched items
		p.batch = &[]T{}
	}
}

func (p *Producer[T]) Put(v T) {
	*p.batch = append(*p.batch, v)
	if len(*p.batch) >= p.parent.batchSize {
		p.Flush()
	}
}

func (p *Producer[T]) NonBlockingPut(v T) bool {
	*p.batch = append(*p.batch, v)
	if len(*p.batch) >= p.parent.batchSize {
		flush := p.NonBlockingFlush()
		return flush
	}
	return true
}

type Consumer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	idx    int
	batch  *[]T
}

func (ch *Chan[T]) Consumer(ctx context.Context) *Consumer[T] {
	result := &Consumer[T]{
		ctx:    ctx,
		parent: ch,
		idx:    0,
		batch:  nil,
	}
	return result
}

func (c *Consumer[T]) prefetch() bool {
	c.idx = 0
	if c.batch != nil && len(*c.batch) > 0 {
		c.parent.putBatchToPool(c.batch)
	}
	c.batch = nil
	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		return ok
	case <-c.ctx.Done():
		return false
	}
}

//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two returned booleans
func (c *Consumer[T]) nonBlockingPrefetch() (readSuccess bool, channelIsOpen bool) {
	c.idx = 0
	if c.batch != nil && len(*c.batch) > 0 {
		c.parent.putBatchToPool(c.batch)
	}
	c.batch = nil
	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		return ok, ok
	default:
		return false, true
	}
}

func (c *Consumer[T]) Poll() (T, bool) {
	if c.batch == nil || c.idx >= len(*c.batch) {
		ok := c.prefetch()
		if !ok {
			return zero[T](), false
		}
	}
	item := (*c.batch)[c.idx]
	c.idx++
	return item, true
}

//nolint:nonamedreturns // here we usenamesreturns to documents meaning of two last returned booleans
func (c *Consumer[T]) NonBlockingPoll() (value T, readSuccess bool, channelIsOpen bool) {
	if c.batch == nil || c.idx >= len(*c.batch) {
		success, open := c.nonBlockingPrefetch()
		if !success {
			return zero[T](), success, open
		}
	}
	item := (*c.batch)[c.idx]
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