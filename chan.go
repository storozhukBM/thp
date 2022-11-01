package thp

import (
	"context"
	"fmt"
	"runtime"
)

type Chan[T any] struct {
	batchSize    int
	internalChan chan []T
}

func NewChan[T any](batchSize int) (*Chan[T], func()) {
	if batchSize < 1 {
		panic("Batch size for thp.Chan can't be lower than 1")
	}
	ch := &Chan[T]{
		batchSize:    batchSize,
		internalChan: make(chan []T, 2*runtime.NumCPU()),
	}
	return ch, ch.Close
}

func (ch *Chan[T]) Close() {
	close(ch.internalChan)
}

func (ch *Chan[T]) Producer(ctx context.Context) (*Producer[T], func()) {
	result := &Producer[T]{
		parent: ch,
		ctx:    ctx,
		batch:  make([]T, 0, ch.batchSize),
	}
	return result, result.Flush
}

type Producer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	batch  []T
}

func (p *Producer[T]) Flush() {
	// TODO: write documentation on how to avoid items dropping
	// in case of ctx cancelation using detached context
	select {
	case p.parent.internalChan <- p.batch:
	case <-p.ctx.Done():
		// we can't block this goroutine anymore
		// and will drop batched items
	}
	fmt.Printf("batch flushed\n")

	p.batch = make([]T, 0, p.parent.batchSize)
}

func (p *Producer[T]) Put(v T) {
	p.batch = append(p.batch, v)
	if len(p.batch) >= p.parent.batchSize {
		p.Flush()
	}
}

type Consumer[T any] struct {
	ctx    context.Context
	parent *Chan[T]
	idx    int
	batch  []T
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
	select {
	case batch, ok := <-c.parent.internalChan:
		c.batch = batch
		return ok
	case <-c.ctx.Done():
		return false
	}
}

func (c *Consumer[T]) Poll() (value T, chAndCtxOpen bool) {
	if c.idx >= len(c.batch) {
		ok := c.prefetch()
		if !ok {
			return zero[T](), ok
		}
	}
	item := c.batch[c.idx]
	c.idx++
	return item, true
}

func (c *Consumer[T]) NonBlockingPoll() (T, bool) {
	return zero[T](), false
}

func zero[T any]() T {
	return *new(T)
}
